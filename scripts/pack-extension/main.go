// scripts/pack-extension — run once before building the main app:
//
//	go run ./scripts/pack-extension
//
// What it does:
//  1. Loads (or generates) an RSA-2048 keypair at build/extension.pem
//  2. Zips extension/ into a CRX3 archive → assets/extension.crx
//  3. Writes the Omaha update manifest   → assets/update_manifest.xml
//  4. Updates the ExtensionID constant in internal/installer/installer.go
package main

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

func main() {
	key := loadOrGenKey("build/extension.pem")
	zipData := mustZip("extension")
	extID := extensionID(key)
	crx := packCRX3(key, zipData)

	must(os.MkdirAll("assets", 0o755))
	must(os.WriteFile("assets/extension.crx", crx, 0o644))
	must(os.WriteFile("assets/update_manifest.xml", updateManifest(extID), 0o644))
	must(patchExtID("internal/installer/installer.go", extID))

	fmt.Printf("✓ Extension ID : %s\n", extID)
	fmt.Println("✓ assets/extension.crx")
	fmt.Println("✓ assets/update_manifest.xml")
	fmt.Println("✓ internal/installer/installer.go — ExtensionID updated")
}

// ---------- key management ----------

func loadOrGenKey(path string) *rsa.PrivateKey {
	if raw, err := os.ReadFile(path); err == nil {
		if b, _ := pem.Decode(raw); b != nil {
			if k, err := x509.ParsePKCS1PrivateKey(b.Bytes); err == nil {
				return k
			}
			if k, err := x509.ParsePKCS8PrivateKey(b.Bytes); err == nil {
				return k.(*rsa.PrivateKey)
			}
		}
	}
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	must(os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
	fmt.Printf("✓ Generated new keypair → %s\n", path)
	return k
}

// extensionID derives the Chrome extension ID from the public key.
// Algorithm: SHA256(DER_SubjectPublicKeyInfo)[0:16], each nibble → 'a'+'n'.
func extensionID(key *rsa.PrivateKey) string {
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	h := sha256.Sum256(der)
	var sb strings.Builder
	for _, b := range h[:16] {
		sb.WriteByte('a' + (b>>4)&0xf)
		sb.WriteByte('a' + b&0xf)
	}
	return sb.String()
}

// ---------- zip ----------

func mustZip(dir string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	must(filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := filepath.ToSlash(must2(filepath.Rel(dir, path)))
		f := must2(w.Create(rel))
		data := must2(os.ReadFile(path))
		_, err = f.Write(data)
		return err
	}))
	must(w.Close())
	return buf.Bytes()
}

// ---------- CRX3 packing ----------
//
// Format: "Cr24" | uint32LE(3) | uint32LE(headerLen) | CrxFileHeader proto | zip
// Signature input: "CRX3 SignedData\x00" | uint32LE(len(signedData)) | signedData | zip

func packCRX3(key *rsa.PrivateKey, zipData []byte) []byte {
	pubDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	h := sha256.Sum256(pubDER)

	// SignedData proto: { crx_id(1): h[:16] }
	signedData := pbLenDelim(1, h[:16])

	// Signature
	sigInput := buildSigInput(signedData, zipData)
	digest := sha256.Sum256(sigInput)
	sig := must2(rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:]))

	// AsymmetricKeyProof proto: { public_key(1): pubDER, signature(2): sig }
	proof := append(pbLenDelim(1, pubDER), pbLenDelim(2, sig)...)

	// CrxFileHeader proto: { sha256_with_rsa(2): proof, signed_header_data(10000): signedData }
	header := append(pbLenDelim(2, proof), pbLenDelim(10000, signedData)...)

	var out bytes.Buffer
	out.WriteString("Cr24")
	_ = binary.Write(&out, binary.LittleEndian, uint32(3))
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(header)))
	out.Write(header)
	out.Write(zipData)
	return out.Bytes()
}

func buildSigInput(signedData, zipData []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("CRX3 SignedData\x00")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(signedData)))
	buf.Write(signedData)
	buf.Write(zipData)
	return buf.Bytes()
}

// ---------- minimal protobuf encoder ----------

func pbVarint(v uint64) []byte {
	var b [10]byte
	n := binary.PutUvarint(b[:], v)
	return b[:n]
}

func pbLenDelim(field int, data []byte) []byte {
	tag := pbVarint(uint64(field<<3 | 2))
	length := pbVarint(uint64(len(data)))
	out := make([]byte, 0, len(tag)+len(length)+len(data))
	out = append(out, tag...)
	out = append(out, length...)
	return append(out, data...)
}

// ---------- update manifest ----------

var manifestTpl = template.Must(template.New("").Parse(
	`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<gupdate xmlns="http://www.google.com/update2/response" protocol="2.0">` + "\n" +
		`  <app appid="{{.ID}}">` + "\n" +
		`    <updatecheck codebase="CRX_PATH_PLACEHOLDER" version="1.0.0"/>` + "\n" +
		`  </app>` + "\n" +
		`</gupdate>` + "\n",
))

func updateManifest(extID string) []byte {
	var buf bytes.Buffer
	must(manifestTpl.Execute(&buf, struct{ ID string }{extID}))
	return buf.Bytes()
}

// ---------- patch ExtensionID in installer.go ----------

func patchExtID(path, extID string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const marker = `const ExtensionID = "`
	s := string(src)
	start := strings.Index(s, marker)
	if start == -1 {
		return fmt.Errorf("ExtensionID constant not found in %s", path)
	}
	start += len(marker)
	end := strings.Index(s[start:], `"`)
	if end == -1 {
		return fmt.Errorf("malformed ExtensionID constant in %s", path)
	}
	return os.WriteFile(path, []byte(s[:start]+extID+s[start+end:]), 0o644)
}

// ---------- helpers ----------

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func must2[T any](v T, err error) T {
	must(err)
	return v
}
