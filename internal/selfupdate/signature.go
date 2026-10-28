package selfupdate

import (
	"errors"
	"fmt"

	"aead.dev/minisign"
)

// releaseKeys are the minisign public keys that sign each release's checksums.txt. A new key goes first, and an old
// key stays until no release that people update to uses it. Anyone can check a release by hand with
// minisign -Vm checksums.txt -P <key>.
var releaseKeys = []string{
	"RWQsuyzz4q/CJYGDrRz9vMHXgekMewZ+HVM3ux70Ft+oaNjrhxfCL+KL",
}

// SignedComment is the trusted comment of a release's signature. It names the version, so the signed checksums of an
// old release cannot pass for a new one.
func SignedComment(version string) string { return fmt.Sprintf("sortdo %s checksums.txt", version) }

// checkRelease accepts checksums.txt only when one of the release keys signed it for this version.
func checkRelease(sums, signature []byte, version string) error {
	keys := make([]minisign.PublicKey, 0, len(releaseKeys))
	for _, text := range releaseKeys {
		var key minisign.PublicKey
		if key.UnmarshalText([]byte(text)) == nil {
			keys = append(keys, key)
		}
	}
	return CheckSignature(sums, signature, version, keys...)
}

// CheckSignature checks that one of keys signed checksums.txt for this version.
func CheckSignature(sums, signature []byte, version string, keys ...minisign.PublicKey) error {
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(signature); err != nil {
		return errors.New("the release's checksums.txt.minisig is not a minisign signature")
	}
	for _, key := range keys {
		if minisign.Verify(key, sums, signature) {
			if parsed.TrustedComment != SignedComment(version) {
				return fmt.Errorf("the release's signature is for %q, not sortdo %s", parsed.TrustedComment, version)
			}
			return nil
		}
	}
	return errors.New("sortdo's release key did not sign this release's checksums.txt")
}
