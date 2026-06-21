package smtp

import (
	"crypto/tls"
	"fmt"
	"strings"

	smtptls "github.com/ehlo-pl/gomailtesttool/internal/smtp/tls"
)

// TLSCSVData holds formatted TLS and certificate data for CSV logging.
type TLSCSVData struct {
	TLSVersion         string
	CipherSuite        string
	CipherStrength     string
	CertSubject        string
	CertIssuer         string
	CertSANs           string // Semicolon-separated list
	CertValidFrom      string // RFC3339 format
	CertValidTo        string // RFC3339 format
	VerificationStatus string
}

// displayComprehensiveTLSInfo displays comprehensive TLS connection and certificate information.
// This function analyzes the TLS state and displays detailed information including cipher suite,
// certificate chain, SANs, and security warnings.
func displayComprehensiveTLSInfo(tlsState *tls.ConnectionState, hostname string, verbose bool) {
	if tlsState == nil {
		if verbose {
			fmt.Println("\nNo TLS connection established")
		}
		return
	}

	// Analyze TLS connection
	tlsInfo := smtptls.AnalyzeTLSConnection(tlsState)
	if tlsInfo != nil {
		displayTLSConnectionInfo(tlsInfo)
	}

	// Analyze certificate chain
	if len(tlsState.PeerCertificates) > 0 {
		certInfo := smtptls.AnalyzeCertificateChain(tlsState.PeerCertificates, hostname)
		if certInfo != nil {
			displayCertificateInfo(certInfo)
		}

		// Check for TLS warnings (skipVerify=false for display purposes)
		warnings := smtptls.CheckTLSWarnings(tlsInfo, certInfo, false)
		if len(warnings) > 0 && verbose {
			fmt.Println("\nSecurity Warnings:")
			fmt.Println(strings.Repeat("═", 60))
			for _, warning := range warnings {
				fmt.Printf("  ⚠ %s\n", warning)
			}
			fmt.Println(strings.Repeat("═", 60))
		}

		// Show recommendations if in verbose mode
		if verbose {
			recommendations := smtptls.GetTLSRecommendations(tlsInfo)
			if len(recommendations) > 0 {
				fmt.Println("\nRecommendations:")
				fmt.Println(strings.Repeat("═", 60))
				for _, rec := range recommendations {
					fmt.Printf("  • %s\n", rec)
				}
				fmt.Println(strings.Repeat("═", 60))
			}
		}
	}
}

// displayTLSConnectionInfo displays TLS connection details.
func displayTLSConnectionInfo(info *smtptls.TLSInfo) {
	if info == nil {
		return
	}

	fmt.Println("\nTLS Connection Details:")
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

// displayCertificateInfo displays certificate details.
func displayCertificateInfo(info *smtptls.CertificateInfo) {
	if info == nil {
		return
	}

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
	} else {
		fmt.Println("  Subject Alternative Names: None")
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

// formatTLSInfoForCSV extracts TLS and certificate information and formats it for CSV output.
// Returns a TLSCSVData struct with all fields populated. If tlsState is nil, returns empty strings.
func formatTLSInfoForCSV(tlsState *tls.ConnectionState, hostname string) TLSCSVData {
	// Return empty data if no TLS state
	if tlsState == nil {
		return TLSCSVData{
			TLSVersion:         "",
			CipherSuite:        "",
			CipherStrength:     "",
			CertSubject:        "",
			CertIssuer:         "",
			CertSANs:           "",
			CertValidFrom:      "",
			CertValidTo:        "",
			VerificationStatus: "",
		}
	}

	// Analyze TLS connection
	tlsInfo := smtptls.AnalyzeTLSConnection(tlsState)

	// Analyze certificate chain
	var certInfo *smtptls.CertificateInfo
	if len(tlsState.PeerCertificates) > 0 {
		certInfo = smtptls.AnalyzeCertificateChain(tlsState.PeerCertificates, hostname)
	}

	// Build CSV data structure
	csvData := TLSCSVData{
		TLSVersion:         "",
		CipherSuite:        "",
		CipherStrength:     "",
		CertSubject:        "",
		CertIssuer:         "",
		CertSANs:           "",
		CertValidFrom:      "",
		CertValidTo:        "",
		VerificationStatus: "",
	}

	// Populate TLS info
	if tlsInfo != nil {
		csvData.TLSVersion = tlsInfo.Version
		csvData.CipherSuite = tlsInfo.CipherSuite
		csvData.CipherStrength = tlsInfo.CipherSuiteStrength
	}

	// Populate certificate info
	if certInfo != nil {
		csvData.CertSubject = certInfo.Subject
		csvData.CertIssuer = certInfo.Issuer
		csvData.CertSANs = strings.Join(certInfo.SANs, ";")
		csvData.CertValidFrom = certInfo.ValidFrom.Format("2006-01-02T15:04:05Z07:00")
		csvData.CertValidTo = certInfo.ValidTo.Format("2006-01-02T15:04:05Z07:00")
		csvData.VerificationStatus = certInfo.VerificationStatus
	}

	return csvData
}
