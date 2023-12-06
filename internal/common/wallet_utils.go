package common

import (
	"encoding/hex"
	"strconv"
	"strings"

	"golang.org/x/crypto/sha3"
)

// ToChecksumAddress converts an Ethereum address to its checksum representation.
func ToChecksumAddress(address string) string {
	// Remove the "0x" prefix and convert to lowercase
	address = strings.Replace(strings.ToLower(address), "0x", "", 1)

	// Calculate the Keccak256 hash of the address
	hash := sha3.NewLegacyKeccak256()
	_, _ = hash.Write([]byte(address))
	sum := hash.Sum(nil)
	digest := hex.EncodeToString(sum)

	// Create a new string builder to store the checksum address
	b := strings.Builder{}
	b.WriteString("0x")

	// Iterate through each character in the address
	for i := 0; i < len(address); i++ {
		// Get the current character
		a := address[i]

		// Check if the character is a letter or a number
		if a > '9' {
			// Get the corresponding digit from the digest
			d, _ := strconv.ParseInt(digest[i:i+1], 16, 8)

			// Check if the digit is greater than or equal to 8
			if d >= 8 {
				// Convert the letter to uppercase
				a -= 'a' - 'A'
				b.WriteByte(a)
			} else {
				// Keep the letter lowercase
				b.WriteByte(a)
			}
		} else {
			// Keep the number as it is
			b.WriteByte(a)
		}
	}

	// Return the checksum address
	return b.String()
}

func FormatUserWallet(wallet string) string {
	return ToChecksumAddress(strings.TrimSpace(strings.ToLower(wallet)))
}

// ToFrontendWallet convert wallet address to frontend required format, currently the requirement is lowercased
func ToFrontendWallet(wallet string) string {
	return strings.ToLower(wallet)
}
