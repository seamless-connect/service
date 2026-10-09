package internal

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

// A domain name is the input of a search, so it has to be a DNS name that can
// be put into a query.
func TestNormalizeDomain(t *testing.T) {
	// Names that have to be accepted, together with what they turn into.
	accepted := map[string]string{
		"example.com":        "example.com",
		"  example.com  ":    "example.com",
		"EXAMPLE.com":        "example.com",
		"example.com.":       "example.com",
		"a.b.example.co.uk":  "a.b.example.co.uk",
		"xn--80ak6aa92e.com": "xn--80ak6aa92e.com",
		"0-9.a-1.com":        "0-9.a-1.com",
	}
	for input, want := range accepted {
		got, err := normalizeDomain(input)
		if err != nil {
			t.Errorf("normalizeDomain(%q) failed: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", input, got, want)
		}
	}

	// Names that have to be rejected.
	rejected := []string{
		"",
		"   ",
		".",
		"example",
		"example.",
		"exa mple.com",
		"-example.com",
		"example-.com",
		"exa_mple.com",
		"example.com/../org",
		"example.com\n.org",
		"exa;mple.com",
		"example..com",
		".example.com",
		// A label is at most 63 characters long.
		strings.Repeat("a", 64) + ".com",
	}
	for _, input := range rejected {
		if got, err := normalizeDomain(input); err == nil {
			t.Errorf("normalizeDomain(%q) = %q, want an error", input, got)
		}
	}
}

// A name that is longer than the 253 characters of a DNS name is not a domain.
func TestNormalizeDomainTooLong(t *testing.T) {
	long := strings.Repeat("aaaaaaaa.", 30)
	if got, err := normalizeDomain(long); err == nil {
		t.Errorf("normalizeDomain() of %d characters = %q, want an error", len(long), got)
	}
}

// The DNS servers of the host are asked, rather than a hard coded list.
func TestResolvConfServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	content := `# a comment
nameserver 127.0.0.53   ; the stub of the host
nameserver 127.0.0.53
nameserver ::1

search example.com
options ndots:1
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("cannot write the resolver configuration: %v", err)
	}

	want := []string{"127.0.0.53:53", "[::1]:53"}
	got := resolvConfServers(path)
	if len(got) != len(want) {
		t.Fatalf("resolvConfServers() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("resolvConfServers()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// A resolver configuration that names no server must not leave the service
// without a DNS server to ask.
func TestResolvConfServersFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("search example.com\n"), 0o600); err != nil {
		t.Fatalf("cannot write the resolver configuration: %v", err)
	}
	if got := resolvConfServers(path); len(got) != len(fallbackDNSServers) {
		t.Errorf("resolvConfServers() = %v, want the fallback %v", got, fallbackDNSServers)
	}
	if got := resolvConfServers(filepath.Join(t.TempDir(), "missing")); len(got) != len(fallbackDNSServers) {
		t.Errorf("resolvConfServers() on a missing file = %v, want the fallback %v", got, fallbackDNSServers)
	}
}

// The answer section must be printed the way dig prints it, because that is
// what the reader of the page is used to.
func TestAnswerSection(t *testing.T) {
	records := []dnsRecord{{
		Name:  "_domainconnect.example.com.",
		TTL:   3600,
		Class: "IN",
		Type:  "TXT",
		Data:  `"api.cloudflare.com/client/v4/dns/domainconnect"`,
	}}
	want := ";; ANSWER SECTION:\n" +
		"_domainconnect.example.com.\t3600\tIN\tTXT\t\"api.cloudflare.com/client/v4/dns/domainconnect\"\n"
	if got := answerSection(records); got != want {
		t.Errorf("answerSection() =\n%q\nwant\n%q", got, want)
	}
}

// The character-strings of a TXT record form one value, see RFC 7208.
func TestTXTRecords(t *testing.T) {
	txt := &dns.TXT{
		Hdr: dns.RR_Header{
			Name:   "_domainconnect.example.com.",
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    3600,
		},
		// A record that is too long for one character-string is sent as
		// several of them.
		Txt: []string{"api.cloudflare.com/client", "/v4/dns/domainconnect"},
	}
	// A CNAME that is not the TXT record of the domain is left out, because
	// the page is about that TXT record.
	cname := &dns.CNAME{
		Hdr:    dns.RR_Header{Name: "_domainconnect.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET},
		Target: "elsewhere.example.net.",
	}
	records := txtRecords(&dns.Msg{Answer: []dns.RR{cname, txt}})

	want := dnsRecord{
		Name:  "_domainconnect.example.com.",
		TTL:   3600,
		Class: "IN",
		Type:  "TXT",
		Data:  `"api.cloudflare.com/client/v4/dns/domainconnect"`,
		value: "api.cloudflare.com/client/v4/dns/domainconnect",
	}
	if len(records) != 1 {
		t.Fatalf("txtRecords() returned %d records, want 1", len(records))
	}
	if records[0] != want {
		t.Errorf("txtRecords() = %+v, want %+v", records[0], want)
	}
}

// The search asks for the TXT record that names the DNS Provider of the domain,
// and returns the answer section of the response.
func TestSearchDNSProvider(t *testing.T) {
	startTestDNSServer(t, "_domainconnect.example.com.",
		[]string{"api.cloudflare.com/client/v4/dns/domainconnect"})

	ctx, cancel := context.WithTimeout(context.Background(), dnsLookupTimeout)
	defer cancel()
	search, err := searchDNSProvider(ctx, "example.com")
	if err != nil {
		t.Fatalf("searchDNSProvider() failed: %v", err)
	}
	want := ";; ANSWER SECTION:\n" +
		"_domainconnect.example.com.\t3600\tIN\tTXT\t\"api.cloudflare.com/client/v4/dns/domainconnect\"\n"
	if search.Answer != want {
		t.Errorf("searchDNSProvider() =\n%q\nwant\n%q", search.Answer, want)
	}
	// The host of the DNS Provider is what the definition is asked for from,
	// so the search has to hand it over rather than only show it.
	if want := "api.cloudflare.com/client/v4/dns/domainconnect"; search.Host != want {
		t.Errorf("searchDNSProvider() host = %q, want %q", search.Host, want)
	}
}

// A domain that has no such record has no DNS Provider, and the caller is told
// about that rather than left with an empty answer.
func TestSearchDNSProviderNoRecord(t *testing.T) {
	// An empty list of texts is a name that exists without a TXT record.
	startTestDNSServer(t, "_domainconnect.example.com.", []string{})

	ctx, cancel := context.WithTimeout(context.Background(), dnsLookupTimeout)
	defer cancel()
	_, err := searchDNSProvider(ctx, "example.com")
	if err == nil {
		t.Fatal("searchDNSProvider() succeeded for a domain without the record")
	}
	if got, want := err.Error(), "_domainconnect.example.com. has no TXT record"; got != want {
		t.Errorf("searchDNSProvider() failed with %q, want %q", got, want)
	}
}

// A search must not hang on when the DNS server does not answer.
func TestSearchDNSProviderTimeout(t *testing.T) {
	// A port that nothing listens on, so that the query is not answered.
	oldServers := dnsServers
	dnsServers = []string{"127.0.0.1:1"}
	defer func() { dnsServers = oldServers }()

	ctx, cancel := context.WithTimeout(context.Background(), dnsLookupTimeout)
	defer cancel()
	if _, err := searchDNSProvider(ctx, "example.com"); err == nil {
		t.Error("searchDNSProvider() against a server that does not answer succeeded")
	}
}

// startTestDNSServer runs a DNS server of its own that answers the TXT
// questions of name with the given texts.  A nil list makes the name unknown,
// an empty one makes it hold no TXT record.  The server is used in place of
// the servers of the host for as long as the test runs.
func startTestDNSServer(t *testing.T, name string, texts []string) {
	t.Helper()
	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot listen for the test DNS server: %v", err)
	}

	handler := dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
		response := new(dns.Msg)
		// A nil list of texts is a name that does not exist, an empty one a
		// name that exists without a TXT record in it.
		if texts == nil || len(request.Question) != 1 || request.Question[0].Name != name {
			response.SetRcode(request, dns.RcodeNameError)
		} else {
			response.SetReply(request)
			if len(texts) > 0 {
				response.Answer = []dns.RR{&dns.TXT{
					Hdr: dns.RR_Header{
						Name:   name,
						Rrtype: dns.TypeTXT,
						Class:  dns.ClassINET,
						Ttl:    3600,
					},
					Txt: texts,
				}}
			}
		}
		if err := writer.WriteMsg(response); err != nil {
			t.Errorf("the test DNS server cannot answer: %v", err)
		}
	})

	server := &dns.Server{PacketConn: connection, Handler: handler}
	started := make(chan struct{})
	server.NotifyStartedFunc = func() { close(started) }
	go func() {
		if err := server.ActivateAndServe(); err != nil {
			t.Errorf("the test DNS server stopped: %v", err)
		}
	}()
	<-started
	t.Cleanup(func() { _ = server.Shutdown() })

	oldServers := dnsServers
	dnsServers = []string{connection.LocalAddr().String()}
	t.Cleanup(func() { dnsServers = oldServers })
}
