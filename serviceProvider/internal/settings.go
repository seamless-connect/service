package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/seamlessdns/service/providerFormats"

	"github.com/rs/zerolog/log"
)

const (
	// settingsPath is put after the host that the TXT record of a domain
	// names, to get the path of the definition of the DNS Provider of the
	// domain.
	settingsPath = "/v2/%s/settings"
	// templateSupportPath is put after the API of a DNS Provider, to ask
	// whether it supports a template and which version of it it has.
	templateSupportPath = "/v2/domainTemplates/providers/%s/services/%s"
	// askTimeout bounds one ask of a DNS Provider, so that a DNS Provider
	// that does not answer cannot hold a request open.  It is carried by
	// the context of the request rather than by the client, because it has
	// to cover reading the body as well.
	// FIXME: make configurable
	askTimeout = 5 * time.Second
	// maxBody is the amount of a response that is read.  What this service
	// asks a DNS Provider is small, so a larger response is not it and is not
	// read into the memory of the service.
	maxBody = 256 << 10
	// maxSnippetLength is the amount of a response that an error message
	// quotes back.  A DNS Provider that answers with a page of its own can
	// answer with a lot, and the error message has to stay readable.
	maxSnippetLength = 200
	// settingsTTL is how long a definition is kept before it is read again.
	// FIXME: follow the cache control of the response instead
	settingsTTL = 5 * time.Minute
)

// reservedFields are the fields of a DNS Provider definition that the
// specification has reserved.  A definition that holds one of them cannot be
// read, and naming the field is more use on the page than the error of the
// parser.
var reservedFields = []string{"urlAsyncUX", "width", "height", "urlControlPanel"}

// settingsClient asks the DNS Providers.  Its connections are kept, because
// the same DNS Provider is asked for every domain it holds.  The bound on a
// single ask is the context of the request rather than the client, because it
// has to cover reading the body as well.
var settingsClient = &http.Client{}

// dnsProviderDefinition is the definition of the DNS Provider of a domain, as
// the DNS Provider itself states it.
type dnsProviderDefinition struct {
	// Provider is the definition as the data structures of this service
	// read it.  It is kept in memory, because the apply of a template needs
	// it and it must not have to be read again for it.
	Provider providerFormats.DNSProvider
	// JSON is the response the definition was read from, as it came.
	JSON string
	// URL is where the definition was read from.
	URL string
	// Expires is when the definition has to be read again.
	Expires time.Time
}

// addressAnswer is what an address of a DNS Provider answered.  The status is
// not turned into an error here, because what a status means differs between
// the things this service asks for.
type addressAnswer struct {
	// Status is the status code of the response.
	Status int
	// Text is the status as it came, "404 Not Found".
	Text string
	// Body is the response body.
	Body []byte
}

// getAddress asks an address of a DNS Provider for its response.  Only the
// ways of failing that no status can stand in for are returned as an error,
// because a response that came is something to report rather than a failure.
func getAddress(ctx context.Context, address string) (addressAnswer, error) {
	log.Debug().Str("url", address).Msg("asking DNS Provider")

	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return addressAnswer{}, fmt.Errorf("cannot ask %s: %w", address, err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := settingsClient.Do(request)
	if err != nil {
		return addressAnswer{}, fmt.Errorf("cannot ask %s: %w", address, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Warn().Str("url", address).Err(err).Msg("could not close http body")
		}
	}()

	// One octet more than the limit is read, so that a response that is over
	// it can be told from one that is exactly it.
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return addressAnswer{}, fmt.Errorf("cannot read the response of %s: %w", address, err)
	}
	if len(body) > maxBody {
		return addressAnswer{}, fmt.Errorf(
			"%s answered more than %d bytes, which is too much", address, maxBody)
	}
	return addressAnswer{Status: response.StatusCode, Text: response.Status, Body: body}, nil
}

// readDefinition reads the definition of the DNS Provider of a domain from the
// host that the TXT record of the domain names, or returns the definition that
// has been read for the domain already.
func readDefinition(ctx context.Context, domain, host string) (dnsProviderDefinition, error) {
	if definition, ok := definitions.get(domain); ok {
		log.Debug().Str("domain", domain).Msg("DNS Provider definition is already in memory")
		return definition, nil
	}
	address, err := settingsURL(host, domain)
	if err != nil {
		return dnsProviderDefinition{}, err
	}
	definition, err := fetchDefinition(ctx, address)
	if err != nil {
		return dnsProviderDefinition{}, err
	}
	definitions.put(domain, definition)
	return definition, nil
}

// settingsURL is the address of the definition of a domain, which is the host
// of the API of the DNS Provider followed by the path of the definition.
//
// The host comes out of DNS, which anyone whose domain this is may write, and
// the address is asked by this service rather than by the browser.  It is
// therefore checked before it is asked: it has to be a host of this service's
// own naming rules, with an optional port and an optional path, and nothing
// else.  A value that carries a scheme, credentials, a query, or a fragment is
// refused rather than passed on to the DNS Provider it names.
func settingsURL(host, domain string) (string, error) {
	if strings.TrimSpace(host) == "" {
		return "", errors.New("the TXT record of the domain is empty")
	}
	address, err := url.Parse("https://" + host)
	if err != nil {
		return "", fmt.Errorf("the TXT record %q does not name a host: %w", host, err)
	}
	if address.Opaque != "" || address.User != nil || address.RawQuery != "" ||
		address.Fragment != "" || address.Host == "" {
		return "", fmt.Errorf("the TXT record %q is not a host", host)
	}
	if _, err := normalizeDomain(address.Hostname()); err != nil {
		return "", fmt.Errorf("the TXT record %q does not name a host: %w", host, err)
	}
	if port := address.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "", fmt.Errorf("the TXT record %q has a port that is not a number", host)
		}
	}
	// The path of the record is the path of the API of the DNS Provider, and
	// the definition is asked for under it.
	return "https://" + host + fmt.Sprintf(settingsPath, url.PathEscape(domain)), nil
}

// fetchDefinition reads a definition of a DNS Provider from an address.  Every
// way it can fail is told apart, because the page reports which one it was.
func fetchDefinition(ctx context.Context, address string) (dnsProviderDefinition, error) {
	answer, err := getAddress(ctx, address)
	if err != nil {
		return dnsProviderDefinition{}, err
	}
	if answer.Status != http.StatusOK {
		return dnsProviderDefinition{}, fmt.Errorf("%s answered %s: %s",
			address, answer.Text, bodySnippet(answer.Body))
	}

	provider, err := parseDefinition(address, answer.Body)
	if err != nil {
		return dnsProviderDefinition{}, err
	}

	// The response is shown as it came, only indented so that it can be read.
	var indented bytes.Buffer
	if err := json.Indent(&indented, answer.Body, "", "  "); err != nil {
		return dnsProviderDefinition{}, fmt.Errorf("%s answered json that cannot be shown: %w", address, err)
	}
	return dnsProviderDefinition{
		Provider: provider,
		JSON:     indented.String(),
		URL:      address,
		Expires:  time.Now().Add(settingsTTL),
	}, nil
}

// parseDefinition reads a definition into the data structures of this service.
// A response that is not json at all, and one that holds a field the
// specification has reserved, are told apart, because the two mean different
// things to whoever put the domain in.
func parseDefinition(address string, body []byte) (providerFormats.DNSProvider, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return providerFormats.DNSProvider{}, fmt.Errorf("%s did not answer json: %w", address, err)
	}
	for _, reserved := range reservedFields {
		if _, ok := fields[reserved]; ok {
			return providerFormats.DNSProvider{}, fmt.Errorf(
				"%s answers the field %q, which the specification has reserved", address, reserved)
		}
	}

	var provider providerFormats.DNSProvider
	if err := json.Unmarshal(body, &provider); err != nil {
		return providerFormats.DNSProvider{}, fmt.Errorf(
			"%s answered json that is not a DNS Provider definition: %w", address, err)
	}
	if provider.ProviderID == "" || provider.URLapi == "" {
		log.Warn().Str("url", address).Msg("DNS Provider definition has no providerId or no urlAPI")
	}
	return provider, nil
}

// readSupportedTemplate asks the DNS Provider of a domain whether it supports
// a template, and returns the version it has of the template.
//
// The providerId and serviceId of the path identify the template itself, so
// they are the ones of the template and not the ones of the DNS Provider.  A
// DNS Provider serves the templates of every Service Provider that has onboarded
// with it, and a template is told apart by who owns it.
func readSupportedTemplate(ctx context.Context, provider providerFormats.DNSProvider, tmpl providerFormats.Template) (providerFormats.SupportedTemplate, error) {
	address, err := templateSupportURL(provider.URLapi, tmpl.ProviderID, tmpl.ServiceID)
	if err != nil {
		return providerFormats.SupportedTemplate{}, err
	}
	answer, err := getAddress(ctx, address)
	if err != nil {
		return providerFormats.SupportedTemplate{}, err
	}

	switch answer.Status {
	case http.StatusNotFound:
		// FIXME: onboarding a template with a DNS Provider is done by hand
		// at the moment, and it should be automated.  Until it is, a
		// template that a DNS Provider answers with 404 for is a template
		// that has to be onboarded, rather than a mistake of the user of
		// the page or of the Service Provider.
		return providerFormats.SupportedTemplate{}, fmt.Errorf(
			"%s answered %s, so the DNS Provider does not support template %s.%s",
			address, answer.Text, tmpl.ProviderID, tmpl.ServiceID)
	case http.StatusOK:
	default:
		return providerFormats.SupportedTemplate{}, fmt.Errorf("%s answered %s: %s",
			address, answer.Text, bodySnippet(answer.Body))
	}

	var supported providerFormats.SupportedTemplate
	if err := json.Unmarshal(answer.Body, &supported); err != nil {
		return providerFormats.SupportedTemplate{}, fmt.Errorf(
			"%s answered json that is not the version of a template: %w", address, err)
	}
	return supported, nil
}

// templateSupportURL is the address that asks a DNS Provider whether it
// supports a template.  The path is put after the API the DNS Provider stated,
// which is where the specification puts it, and the two identifiers in it are
// the ones of the template rather than the ones of the DNS Provider.
//
// The API comes out of the settings of the DNS Provider, which in turn came out
// of a TXT record.  It is therefore checked rather than asked as it stands.
func templateSupportURL(urlAPI, providerID, serviceID string) (string, error) {
	if urlAPI == "" {
		return "", errors.New("the DNS Provider does not state the address of its API")
	}
	if providerID == "" {
		return "", errors.New("the template does not state which provider it belongs to")
	}
	if serviceID == "" {
		return "", errors.New("the template does not state which service it is")
	}
	base, err := url.Parse(urlAPI)
	if err != nil {
		return "", fmt.Errorf("the API %q of the DNS Provider is not an address: %w", urlAPI, err)
	}
	if base.Scheme != "https" && base.Scheme != "http" {
		return "", fmt.Errorf("the API %q of the DNS Provider is not an http address", urlAPI)
	}
	if base.Host == "" || base.User != nil {
		return "", fmt.Errorf("the API %q of the DNS Provider names no host", urlAPI)
	}
	return strings.TrimSuffix(urlAPI, "/") + fmt.Sprintf(templateSupportPath,
		url.PathEscape(providerID), url.PathEscape(serviceID)), nil
}

// bodySnippet is a short piece of a response, for an error message to tell what
// the endpoint answered.  A DNS Provider can answer with anything, so the
// piece is cut to a length that can be read.
func bodySnippet(body []byte) string {
	// The cut can land inside a character, and the message is text.
	text := strings.ToValidUTF8(string(body), "")
	text = strings.TrimSpace(text)
	if len(text) > maxSnippetLength {
		return strings.ToValidUTF8(text[:maxSnippetLength], "") + "..."
	}
	return text
}

// definitions keeps the definitions that have been read.  A definition is held
// in the memory of the service, because the apply of a template needs it and it
// must not have to be asked for again.
var definitions = newDefinitionCache(settingsTTL)

// definitionCache holds the definition of the DNS Provider of a domain until it
// expires.  It is safe to use from the request handlers at the same time.
type definitionCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]dnsProviderDefinition
}

func newDefinitionCache(ttl time.Duration) *definitionCache {
	return &definitionCache{ttl: ttl, entries: make(map[string]dnsProviderDefinition)}
}

// get returns the definition of a domain when one is in memory and has not
// expired.
func (cache *definitionCache) get(domain string) (dnsProviderDefinition, bool) {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	definition, ok := cache.entries[domain]
	if !ok {
		return dnsProviderDefinition{}, false
	}
	if time.Now().After(definition.Expires) {
		return dnsProviderDefinition{}, false
	}
	return definition, true
}

// put holds the definition of a domain in memory for the time to live of the
// cache.
func (cache *definitionCache) put(domain string, definition dnsProviderDefinition) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	definition.Expires = time.Now().Add(cache.ttl)
	cache.entries[domain] = definition
}
