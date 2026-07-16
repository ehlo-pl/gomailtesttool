package tls

import (
	"fmt"
	"strings"
)

// PrintTLSInfo displays TLS connection details on stdout.
func PrintTLSInfo(info *TLSInfo) {
	fmt.Println("TLS Connection Details:")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("  Protocol Version:    %s\n", info.Version)
	fmt.Printf("  Cipher Suite:        %s\n", info.CipherSuite)
	fmt.Printf("  Cipher Strength:     %s\n", strings.ToUpper(info.CipherSuiteStrength))
	if info.ServerName != "" {
		fmt.Printf("  Server Name (SNI):   %s\n", info.ServerName)
	}
	if info.NegotiatedProtocol != "" {
		fmt.Printf("  Negotiated Protocol: %s\n", info.NegotiatedProtocol)
	}
	fmt.Println(strings.Repeat("═", 60))
}

// PrintCertificateInfo displays certificate details on stdout.
func PrintCertificateInfo(info *CertificateInfo) {
	fmt.Println("\nCertificate Information:")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("  Subject:             %s\n", info.Subject)
	fmt.Printf("  Issuer:              %s\n", info.Issuer)
	fmt.Printf("  Serial Number:       %s\n", info.SerialNumber)
	fmt.Printf("  Valid From:          %s\n", info.ValidFrom.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Valid To:            %s\n", info.ValidTo.Format("2006-01-02 15:04:05 MST"))

	if info.IsExpired {
		fmt.Printf("  Status:              ⚠ EXPIRED\n")
	} else {
		fmt.Printf("  Days Until Expiry:   %d\n", info.DaysUntilExpiry)
	}

	if len(info.SANs) > 0 {
		fmt.Println("  Subject Alternative Names:")
		for _, san := range info.SANs {
			fmt.Printf("    • %s\n", san)
		}
	}

	fmt.Printf("  Signature Algorithm: %s\n", info.SignatureAlgorithm)
	fmt.Printf("  Public Key:          %s (%d bits)\n", info.PublicKeyAlgorithm, info.PublicKeySize)

	if len(info.KeyUsage) > 0 {
		fmt.Printf("  Key Usage:           %s\n", strings.Join(info.KeyUsage, ", "))
	}
	if len(info.ExtKeyUsage) > 0 {
		fmt.Printf("  Extended Key Usage:  %s\n", strings.Join(info.ExtKeyUsage, ", "))
	}

	fmt.Printf("  Verification:        %s\n", strings.ToUpper(info.VerificationStatus))
	fmt.Printf("  Chain Length:        %d certificate(s)\n", info.ChainLength)

	if info.IsSelfSigned {
		fmt.Println("  ⚠ Self-signed certificate")
	}

	fmt.Println(strings.Repeat("═", 60))
}

// PrintTLSWarnings displays TLS warnings on stdout (no-op for an empty list).
func PrintTLSWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Println("\n⚠ TLS Warnings:")
	fmt.Println(strings.Repeat("─", 60))
	for _, w := range warnings {
		fmt.Printf("  • %s\n", w)
	}
	fmt.Println(strings.Repeat("─", 60))
}

// PrintTLSRecommendations displays TLS recommendations on stdout (no-op for an
// empty list).
func PrintTLSRecommendations(recommendations []string) {
	if len(recommendations) == 0 {
		return
	}
	fmt.Println("\n💡 Recommendations:")
	fmt.Println(strings.Repeat("─", 60))
	for _, r := range recommendations {
		fmt.Printf("  • %s\n", r)
	}
	fmt.Println(strings.Repeat("─", 60))
}
