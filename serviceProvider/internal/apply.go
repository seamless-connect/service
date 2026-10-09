package internal

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/seamlessdns/service/providerFormats"

	"github.com/rs/zerolog/log"
)

const (
	// applyPath is put after the UX of the DNS Provider, to get the address
	// the user is sent to in order to apply a template.
	applyPath = "/v2/domainTemplates/providers/%s/services/%s/apply"
	// sigParameter and keyParameter are the last two parameters of an apply
	// request.  They are not part of what is signed, because they are the
	// signature itself and where to find the key that made it.
	sigParameter = "sig"
	keyParameter = "key"
)

// queryParameter is one parameter of an apply request.
type queryParameter struct {
	Name  string
	Value string
}

// applyPageData fills the fragment with the request the user can make.
type applyPageData struct {
	Domain string
	// Description tells which template the request applies.
	Description string
	// Command is the curl command that makes the request.  It is shown and
	// not run: applying a template changes DNS, and that is the user's call.
	Command string
	// Post is the data a DNS Provider posts on to verify the signature.
	Post string
	// Error tells why the request could not be built, if it could not.
	Error string
}

// curlRequest builds the apply request of a chosen template from the values the
// user gave, and shows it.  Nothing is sent to the DNS Provider: the request is
// what the user makes, once they have read it.
func curlRequest(c *gin.Context, tmpls Templates) {
	domain, err := normalizeDomain(c.Query("domain"))
	if err != nil {
		renderFragment(c, pagesTemplate.Lookup("curlRequest"), applyPageData{Error: err.Error()})
		return
	}
	id := c.Query("template")
	conf, ok := tmpls[id]
	if !ok {
		renderFragment(c, pagesTemplate.Lookup("curlRequest"),
			applyPageData{Domain: domain, Error: fmt.Sprintf("template %q is not configured", id)})
		return
	}
	data := applyPageData{Domain: domain, Description: conf.Template.ProviderID + "." + conf.Template.ServiceID}

	// What the DNS Provider is to be asked, and where the user is sent, both
	// come from the definition that the search of the domain read.
	definition, ok := definitions.get(domain)
	if !ok {
		data.Error = fmt.Sprintf(
			"what the DNS Provider of %s supports is not known, because its settings could not be read, so the request cannot be built", domain)
		renderFragment(c, pagesTemplate.Lookup("curlRequest"), data)
		return
	}
	// The DNS Provider states urlSyncUX only when it has a synchronous flow,
	// and applying a template is the synchronous flow.
	if definition.Provider.URLSyncUX == "" {
		data.Error = fmt.Sprintf("the DNS Provider of %s does not offer a synchronous flow, so a template cannot be applied with it", domain)
		renderFragment(c, pagesTemplate.Lookup("curlRequest"), data)
		return
	}

	group := c.Query("group")
	if len(templateGroups(conf.Template)) > 0 && group == "" {
		// A template that groups its records is applied one group at a
		// time, so there is no single request to build until the group is
		// known.
		data.Error = "the template groups its records, so the group to apply has to be chosen first"
		renderFragment(c, pagesTemplate.Lookup("curlRequest"), data)
		return
	}

	// Only the variables that the template declares are read from the form,
	// so that whatever else was put into the request cannot end up in what is
	// signed.
	query := applyQuery(domain, group, conf.Template, c.PostForm)
	signature, err := signQuery(conf, query)
	if err != nil {
		log.Warn().Err(err).Str("domain", domain).Str("template", data.Description).
			Msg("apply request cannot be signed")
		data.Error = err.Error()
		renderFragment(c, pagesTemplate.Lookup("curlRequest"), data)
		return
	}

	address, err := applyURL(definition.Provider.URLSyncUX, conf.Template.ProviderID, conf.Template.ServiceID, query, signature, conf.PubKeyHost)
	if err != nil {
		data.Error = err.Error()
		renderFragment(c, pagesTemplate.Lookup("curlRequest"), data)
		return
	}
	data.Command = curlCommand(address)
	data.Post = postDataJSON(conf, query, signature)
	renderFragment(c, pagesTemplate.Lookup("curlRequest"), data)
}

// applyQuery is what the signature is made over: the domain, the group, and the
// variables of the records being applied.  The two parameters that carry the
// signature itself and where to find the key are left out, because they cannot
// be part of what they carry.
//
// values are read one name at a time, and only for the names the template
// declares, so that whatever else was put into the form cannot end up in what
// is signed.
func applyQuery(domain, group string, tmpl providerFormats.Template, values func(string) string) string {
	query := []queryParameter{{Name: "domain", Value: domain}}
	for _, variable := range templateVariables(tmpl, group) {
		query = append(query, queryParameter{Name: variable.Name, Value: values(variable.Name)})
	}
	// The records to apply are told apart by group only when the template uses
	// groups.  Without a group the whole template is applied, and saying so
	// is not needed.
	if group != "" {
		query = append(query, queryParameter{Name: "groupId", Value: group})
	}
	return joinQuery(query)
}

// joinQuery encodes the parameters the way they go into the address.  The
// signature is made over exactly this, so the DNS Provider arrives at the same
// string from the request it received.
func joinQuery(query []queryParameter) string {
	var joined strings.Builder
	for i, parameter := range query {
		if i > 0 {
			joined.WriteByte('&')
		}
		joined.WriteString(url.QueryEscape(parameter.Name))
		joined.WriteByte('=')
		joined.WriteString(url.QueryEscape(parameter.Value))
	}
	return joined.String()
}

// applyURL is the address the user is sent to.  It carries the query that was
// signed, and then the signature and where to find the key that made it.
func applyURL(urlSyncUX, providerID, serviceID, query, signature, pubKeyHost string) (string, error) {
	if urlSyncUX == "" {
		return "", errors.New("the DNS Provider does not offer an address to apply a template at")
	}
	base, err := url.Parse(urlSyncUX)
	if err != nil {
		return "", fmt.Errorf("the apply address %q of the DNS Provider is not an address: %w", urlSyncUX, err)
	}
	if base.Scheme != "https" && base.Scheme != "http" {
		return "", fmt.Errorf("the apply address %q of the DNS Provider is not an http address", urlSyncUX)
	}
	if base.Host == "" || base.User != nil {
		return "", fmt.Errorf("the apply address %q of the DNS Provider names no host", urlSyncUX)
	}
	if providerID == "" || serviceID == "" {
		return "", errors.New("the template does not state which provider and service it is")
	}

	full := query
	if full != "" {
		full += "&"
	}
	full += url.QueryEscape(sigParameter) + "=" + url.QueryEscape(signature)
	full += "&" + url.QueryEscape(keyParameter) + "=" + url.QueryEscape(pubKeyHost)

	return strings.TrimSuffix(urlSyncUX, "/") +
		fmt.Sprintf(applyPath, url.PathEscape(providerID), url.PathEscape(serviceID)) +
		"?" + full, nil
}

// curlCommand is the command that makes the request, quoted so that a shell
// takes it as it stands.
func curlCommand(address string) string {
	return "curl -v " + shellQuote(address)
}

// shellQuote wraps text in single quotes for a shell, ending and reopening a
// quote around the single quotes the text has of its own.
func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// postDataJSON is the data that is posted to a DNS Provider to verify the
// signature, in the shape the specification has.
//
// The domain of the post data is not the domain that is being changed.  It is
// the DNS name that holds the public key, because that is where the key that
// made the signature is looked up.
func postDataJSON(conf Template, query, signature string) string {
	post := providerFormats.PostData{
		Domain: pubKeyName(conf),
		Sig:    signature,
		Hash:   query,
	}
	// The encoder escapes the ampersands of the query, which is right for
	// html but not here: what is shown has to be the data that goes on the
	// wire, and the html escaping of the page takes care of the markup.
	var marshaled bytes.Buffer
	encoder := json.NewEncoder(&marshaled)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(post); err != nil {
		log.Error().Err(err).Msg("could not write the post data")
		return ""
	}
	// The encoder ends what it writes with a newline of its own.
	return strings.TrimRight(marshaled.String(), "\n")
}

// pubKeyName is the DNS name that holds the public key of a template, which is
// the host of the configuration in front of the domain of the template.
func pubKeyName(conf Template) string {
	if conf.PubKeyHost == "" || conf.Template.SyncPubKeyDomain == "" {
		return ""
	}
	return conf.PubKeyHost + "." + conf.Template.SyncPubKeyDomain
}

// signQuery signs the query of an apply request with the key of a template.
// The algorithm is the one the configuration states, which is the same one the
// public key is published with, so that the DNS Provider can check it.
func signQuery(conf Template, query string) (string, error) {
	if conf.SigningKey == nil {
		return "", errors.New("the template has no signing key, so the request cannot be signed")
	}
	if conf.Template.SyncPubKeyDomain == "" {
		return "", errors.New("the template does not state a syncPubKeyDomain, so the request cannot be signed")
	}
	if conf.PubKeyHost == "" {
		return "", errors.New("the template has no pubKeyHost in the configuration, so the DNS Provider cannot find the public key")
	}

	switch key := conf.SigningKey.(type) {
	case *rsa.PrivateKey:
		digest, hash, err := digestFor(conf.SecretType, query)
		if err != nil {
			return "", err
		}
		var signature []byte
		if hash == crypto.SHA256 && strings.HasPrefix(conf.SecretType, "PS") {
			signature, err = rsa.SignPSS(rand.Reader, key, hash, digest, nil)
		} else {
			signature, err = rsa.SignPKCS1v15(rand.Reader, key, hash, digest)
		}
		if err != nil {
			return "", fmt.Errorf("the request could not be signed: %w", err)
		}
		return base64.StdEncoding.EncodeToString(signature), nil

	case *ecdsa.PrivateKey:
		digest, _, err := digestFor(conf.SecretType, query)
		if err != nil {
			return "", err
		}
		signature, err := ecdsa.SignASN1(rand.Reader, key, digest)
		if err != nil {
			return "", fmt.Errorf("the request could not be signed: %w", err)
		}
		return base64.StdEncoding.EncodeToString(signature), nil

	case ed25519.PrivateKey:
		if conf.SecretType != "Ed25519" {
			return "", fmt.Errorf("the signing key is an Ed25519 key but the configuration states %q", conf.SecretType)
		}
		// Ed25519 signs the message as it is, the hash being part of the
		// algorithm rather than a step before it.
		return base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(query))), nil

	default:
		return "", fmt.Errorf("the signing key is a %T, which is not a key a request can be signed with", conf.SigningKey)
	}
}

// digestFor is the digest of the query under the algorithm the configuration
// states, together with the hash the signature is made with.
func digestFor(secretType, query string) ([]byte, crypto.Hash, error) {
	hash, err := hashFor(secretType)
	if err != nil {
		return nil, 0, err
	}
	var digest []byte
	switch hash {
	case crypto.SHA384:
		d := sha512.Sum384([]byte(query))
		digest = d[:]
	case crypto.SHA512:
		d := sha512.Sum512([]byte(query))
		digest = d[:]
	default:
		d := sha256.Sum256([]byte(query))
		digest = d[:]
	}
	return digest, hash, nil
}

// hashFor is the hash that goes with an algorithm name of the configuration.
// The names are the ones the specification publishes the public key with.
func hashFor(secretType string) (crypto.Hash, error) {
	switch secretType {
	case "RS256", "PS256", "ES256":
		return crypto.SHA256, nil
	case "RS384", "PS384", "ES384":
		return crypto.SHA384, nil
	case "RS512", "PS512", "ES512":
		return crypto.SHA512, nil
	default:
		return 0, fmt.Errorf("%q is not an algorithm a request can be signed with", secretType)
	}
}

// curlRequestHandler binds the templates of the configuration to the handler
// that builds the apply request.
func curlRequestHandler(tmpls Templates) gin.HandlerFunc {
	return func(c *gin.Context) { curlRequest(c, tmpls) }
}
