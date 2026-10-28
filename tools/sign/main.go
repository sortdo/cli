// sign signs a release's checksums.txt with sortdo's minisign release key. The release workflow runs it through
// GoReleaser; sortdo update checks the signature before it trusts any checksum.
//
//	go run ./tools/sign <checksums.txt> <signature file> <version>
//
// SORTDO_SIGNING_KEY holds the secret key file (as minisign -G writes it) and SORTDO_SIGNING_PASSWORD its password.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"aead.dev/minisign"
	"github.com/sortdo/cli/internal/selfupdate"
)

func main() {
	if len(os.Args) != 4 {
		fail("Usage: sign <checksums.txt> <signature file> <version>")
	}
	file, signatureFile, version := os.Args[1], os.Args[2], os.Args[3]
	secret := []byte(os.Getenv("SORTDO_SIGNING_KEY"))
	if len(bytes.TrimSpace(secret)) == 0 {
		fail("SORTDO_SIGNING_KEY is not set. A release must be signed, or sortdo update refuses it.")
	}
	var key minisign.PrivateKey
	var err error
	if minisign.IsEncrypted(secret) {
		key, err = minisign.DecryptKey(os.Getenv("SORTDO_SIGNING_PASSWORD"), secret)
	} else {
		err = key.UnmarshalText(secret)
	}
	if err != nil {
		fail(fmt.Sprintf("Could not read the signing key: %v", err))
	}
	sums, err := os.ReadFile(file)
	if err != nil {
		fail(err.Error())
	}
	// The prehashed form (BLAKE2b, then Ed25519) is what current minisign versions make and expect.
	reader := minisign.NewReader(bytes.NewReader(sums))
	if _, err := io.Copy(io.Discard, reader); err != nil {
		fail(err.Error())
	}
	signature := reader.SignWithComments(key, selfupdate.SignedComment(version), "signature from the sortdo release key")
	if err := selfupdate.CheckSignature(sums, signature, version, key.Public().(minisign.PublicKey)); err != nil {
		fail(fmt.Sprintf("The new signature does not check out: %v", err))
	}
	if err := os.WriteFile(signatureFile, signature, 0o644); err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "sign:", message)
	os.Exit(1)
}
