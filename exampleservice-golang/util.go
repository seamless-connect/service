package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	messageRegex = regexp.MustCompile(`^[a-zA-Z ,.?!0-9]*$`)
)

func dnsMessageData(message string) string {
	secs := time.Now().UTC().Unix()
	return fmt.Sprintf("shm:%d:%s", secs, message)
}

func isValidHostname(hostname string) bool {
	if len(hostname) < 4 || len(hostname) > 255 {
		return false
	}
	hostname = strings.TrimSuffix(hostname, ".")
	for _, part := range strings.Split(hostname, ".") {
		if len(part) < 1 || len(part) > 63 {
			return false
		}
		if part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for i := 0; i < len(part); i++ {
			c := part[i]
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}

func isValidMessage(message string) bool {
	if len(message) < 1 || len(message) > 255 {
		return false
	}
	return messageRegex.MatchString(message)
}

func getMessageText(domain string) string {
	answers, err := net.LookupTXT(domain)
	if err != nil {
		return ""
	}

	timestamps := make(map[int64]string)
	for _, answer := range answers {
		data := strings.Split(answer, ":")
		if len(data) >= 2 && data[0] == "shm" {
			if len(data) == 2 {
				data = append([]string{"shm", "0"}, data[1:]...)
			}
			if len(data) >= 3 {
				date, err := strconv.ParseInt(data[1], 10, 64)
				if err == nil {
					timestamps[date] = data[2]
				}
			}
		}
	}

	var messagetext string
	var dates []int64
	for date := range timestamps {
		dates = append(dates, date)
	}
	// sort keys
	for i := 0; i < len(dates); i++ {
		for j := i + 1; j < len(dates); j++ {
			if dates[i] > dates[j] {
				dates[i], dates[j] = dates[j], dates[i]
			}
		}
	}
	for _, date := range dates {
		messagetext += timestamps[date] + "<br>"
	}
	return messagetext
}

func checkTemplate(url string) bool {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

type DomainConnectSettings struct {
	ProviderName string   `json:"providerName"`
	URLAPI       string   `json:"urlAPI"`
	URLSyncUX    string   `json:"urlSyncUX"`
	URLAsyncUX   string   `json:"urlAsyncUX"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	NameServers  []string `json:"nameServers"`
}

func getDomainConnectJSON(domain string) (*DomainConnectSettings, string, string) {
	answers, err := net.LookupTXT("_domainconnect." + domain)
	if err != nil || len(answers) != 1 {
		return nil, "", "No _domainconnect TXT record"
	}
	host := answers[0]

	url := "https://" + host + "/v2/" + domain + "/settings"
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", "No json returned for /settings"
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", "No json returned for /settings"
	}

	var jsonData DomainConnectSettings
	if err := json.NewDecoder(resp.Body).Decode(&jsonData); err != nil {
		return nil, "", "Internal error"
	}

	if len(jsonData.NameServers) > 0 {
		nsRecords, err := net.LookupNS(domain)
		if err != nil || len(nsRecords) == 0 {
			return nil, "", "No nameservers found"
		}
		authoritative := strings.TrimSuffix(nsRecords[0].Host, ".")
		found := false
		for _, ns := range jsonData.NameServers {
			if ns == authoritative {
				found = true
				break
			}
		}
		if !found {
			return nil, "", "Nameservers not authoritative"
		}
	}

	return &jsonData, host, ""
}
