package imap

import "errors"

// xoauth2Client is the SASL XOAUTH2 mechanism used by Outlook.com, Microsoft
// 365 and Gmail. go-sasl only offers OAUTHBEARER (RFC 7628), a different
// mechanism that Exchange Online does not accept.
type xoauth2Client struct {
	username, token string
}

// Start returns the initial response "user=U^Aauth=Bearer T^A^A".
func (a *xoauth2Client) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.username + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

// Next is only called when the server rejects the token: it sends a JSON
// error as a challenge.
func (a *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	return nil, errors.New("XOAUTH2 rejected by server: " + string(challenge))
}
