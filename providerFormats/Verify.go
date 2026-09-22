package providerFormats

import (
	"crypto"
)

// PostData contains data sent to /v2/verify end point.
type PostData struct {
	Domain string `json:"domain"`
	Sig    string `json:"sig"`
	Hash   string `json:"hash"`
}

// SPPubKeyTXT contains the Service Provider public key that a DNS Provider is using
// to validate if the update was signed by the template syncPubKeyDomain key.
type SPPubKeyTXT struct {
	Position int    `json:"p"`
	Data     string `json:"d"`
	Alg      string `json:"a,omitempty"`
	Type     string `json:"t,omitempty"`
}

// SigScheme identifies the signature algorithm family.
type SigScheme int

const (
	SchemeUnknown  SigScheme = iota
	SchemeRSAPKCS1           // RS256, RS384, RS512
	SchemeRSAPSS             // PS256, PS384, PS512
	SchemeECDSA              // ES256, ES384, ES512
	SchemeEd25519            // Ed25519
)

// SPPubKey is parsed and processed SPPubKeyTXT entry, that can be used by
// crypto algorithms.  These entries can be cached.
type SPPubKey struct {
	Key    crypto.PublicKey
	Scheme SigScheme
	Hash   crypto.Hash // not used for Ed25519
}
