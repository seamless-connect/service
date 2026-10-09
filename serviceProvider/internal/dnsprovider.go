package internal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/rs/zerolog/log"
)

// domainConnectPrefix is the label that is put in front of the domain of the
// user to get the name of the TXT record that names the DNS Provider of the
// domain.
const domainConnectPrefix = "_domainconnect."

const (
	// dnsLookupTimeout bounds a search, so that a DNS server that does not
	// answer cannot hold a request open.
	// FIXME: make configurable
	dnsLookupTimeout = 5 * time.Second
	// dnsPort is the port the DNS servers are asked on.
	dnsPort = "53"
	// resolvConfPath is the resolver configuration of the host.  Its
	// nameservers are the ones the searches are sent to.
	resolvConfPath = "/etc/resolv.conf"
)

// fallbackDNSServers are asked when the resolver configuration of the host
// does not name a server.
var fallbackDNSServers = []string{"1.1.1.1:53", "8.8.8.8:53"}

// dnsServers are the servers that the searches are sent to, in host:port form.
// The tests replace it with a server of their own.
var dnsServers = resolvConfServers(resolvConfPath)

// dnsRecord is one record of the answer section of a DNS response, in the form
// that it is shown on the page.
type dnsRecord struct {
	Name  string
	TTL   uint32
	Class string
	Type  string
	Data  string
	// value is the record data without the quotes that Data puts around it.
	// It is kept so that the host of the DNS Provider can be used, rather
	// than parsed back out of what the page shows.
	value string
}

// String formats the record the way dig prints a record of an answer section.
func (record dnsRecord) String() string {
	return fmt.Sprintf("%s\t%d\t%s\t%s\t%s", record.Name, record.TTL, record.Class, record.Type, record.Data)
}

// answerSection formats the records the way dig prints the answer section of a
// response.
func answerSection(records []dnsRecord) string {
	var section strings.Builder
	section.WriteString(";; ANSWER SECTION:\n")
	for _, record := range records {
		section.WriteString(record.String() + "\n")
	}
	return section.String()
}

// providerSearch is what the _domainconnect.<domain> TXT record of a domain
// states: the answer section as dig prints it, and the host that the record
// names.
type providerSearch struct {
	// Answer is the answer section of the response.
	Answer string
	// Host is the value of the TXT record, which is the host and path of
	// the API of the DNS Provider of the domain.
	Host string
}

// searchDNSProvider searches the _domainconnect.<domain> TXT record, which
// names the DNS Provider of the domain.
func searchDNSProvider(ctx context.Context, domain string) (providerSearch, error) {
	if len(dnsServers) == 0 {
		return providerSearch{}, errors.New("no DNS server is configured")
	}
	name := dns.Fqdn(domainConnectPrefix + domain)
	log.Debug().Str("domain", domain).Msg("searching DNS Provider")

	query := new(dns.Msg)
	query.SetQuestion(name, dns.TypeTXT)
	client := new(dns.Client)

	// The servers are asked in turn, and the first one that answers decides
	// the outcome of the search.
	var failures []error
	for _, server := range dnsServers {
		response, _, err := client.ExchangeContext(ctx, query, server)
		if err != nil {
			log.Debug().Err(err).Str("server", server).Msg("DNS server did not answer")
			failures = append(failures, err)
			continue
		}
		switch response.Rcode {
		case dns.RcodeSuccess:
		case dns.RcodeNameError:
			return providerSearch{}, fmt.Errorf("%s does not exist", name)
		default:
			return providerSearch{}, fmt.Errorf("%s: DNS server answered %s", name, dns.RcodeToString[response.Rcode])
		}
		records := txtRecords(response)
		if len(records) == 0 {
			return providerSearch{}, fmt.Errorf("%s has no TXT record", name)
		}
		// The record holds the host of the API of the DNS Provider.  More
		// than one of them is a domain this service cannot act on, and
		// picking one of them would send the definition to an address that
		// the domain does not state.
		if len(records) > 1 {
			return providerSearch{}, fmt.Errorf(
				"%s has %d TXT records, so the DNS Provider of the domain is not known", name, len(records))
		}
		return providerSearch{Answer: answerSection(records), Host: records[0].value}, nil
	}
	return providerSearch{}, fmt.Errorf("cannot search %s: %w", name, errors.Join(failures...))
}

// txtRecords collects the TXT records of the answers of a DNS response.  Other
// records, such as the CNAMEs of a redirection chain, are left out, because the
// page is about the TXT record of the domain.
func txtRecords(response *dns.Msg) []dnsRecord {
	var records []dnsRecord
	for _, answer := range response.Answer {
		txt, ok := answer.(*dns.TXT)
		if !ok {
			continue
		}
		header := txt.Hdr
		// A TXT record holds a list of character-strings that are
		// concatenated into one value, see RFC 7208.
		value := strings.Join(txt.Txt, "")
		records = append(records, dnsRecord{
			Name:  header.Name,
			TTL:   header.Ttl,
			Class: dns.ClassToString[header.Class],
			Type:  dns.TypeToString[header.Rrtype],
			Data:  strconv.Quote(value),
			value: value,
		})
	}
	return records
}

// resolvConfServers returns the nameservers of the resolver configuration file
// in host:port form.  The public fallback servers are returned when the file
// names none, so that a host without a resolver configuration can still make
// the searches.
func resolvConfServers(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("cannot read resolver configuration")
		return fallbackDNSServers
	}
	defer file.Close()

	var servers []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Both '#' and ';' start a comment.
		if comment := strings.IndexAny(line, "#;"); comment >= 0 {
			line = line[:comment]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		server := net.JoinHostPort(fields[1], dnsPort)
		if seen[server] {
			continue
		}
		seen[server] = true
		servers = append(servers, server)
	}
	if err := scanner.Err(); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("cannot read resolver configuration")
	}
	if len(servers) == 0 {
		log.Warn().Str("path", path).Msg("resolver configuration has no nameserver")
		return fallbackDNSServers
	}
	log.Debug().Str("servers", strings.Join(servers, ",")).Msg("DNS servers in use")
	return servers
}
