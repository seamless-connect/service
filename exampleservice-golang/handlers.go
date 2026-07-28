package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Template data types
type BaseData struct {
	Title string
}

type IndexData struct {
	BaseData
}

type SimplePostData struct {
	BaseData
	ProviderName    string
	Width           int
	Height          int
	SynchronousURL1 string
	FQDN            string
}

type SyncPostData struct {
	BaseData
	TXT                        string
	JSON                       template.JS
	CheckURL1                  string
	CheckURL2                  string
	Domain                     string
	ProviderName               string
	Width                      int
	Height                     int
	SynchronousURL1            string
	SynchronousSignedURL2      string
	QS                         string
	Sig                        string
	SynchronousRedirectURL1    string
	SynchronousSignedRedirectURL2 string
	QSRedirect                 string
	SigRedirect                string
}

type SyncConfirmData struct {
	BaseData
	Domain    string
	Subdomain string
}

type SyncErrorData struct {
	BaseData
	Error string
}

type AsyncPostData struct {
	BaseData
	TXT            string
	JSON           template.JS
	CheckURL1      string
	CheckURL2      string
	Domain         string
	Hosts          string
	ProviderName   string
	AsynchronousURL string
}

type AsyncOAuthResponseData struct {
	BaseData
	Code         string
	URL          string
	Domain       string
	Hosts        string
	DNSProvider  string
	AccessToken  string
	JSONResponse template.JS
}

type AsyncConfirmData struct {
	BaseData
	AppliedTemplate string
	URL             string
	Message         string
	AccessToken     string
	Domain          string
	Subdomain       string
	Hosts           string
	DNSProvider     string
	StatusCode      string
}

type AsyncErrorData struct {
	BaseData
	Error string
}

type SigGenerateData struct {
	BaseData
	Sig string
}

type SigFetchData struct {
	BaseData
	Domain        string
	Key           string
	PubKey        string
	RecordStrings []string
}

type SigVerifyData struct {
	BaseData
	Domain        string
	Key           string
	Sig           string
	QS            string
	Verified      bool
	PubKey        string
	RecordStrings []string
}

type DDNSCodeData struct {
	BaseData
	OAuthCode string
}

type DDNSErrorData struct {
	BaseData
	Error string
}

type SiteData struct {
	Host        string
	MessageText template.HTML
}

type NoDomainConnectData struct {
	BaseData
	Reason string
}

type InvalidData struct {
	BaseData
}

var tmpl *template.Template

func loadTemplates() {
	funcMap := template.FuncMap{
		"safeJS": func(s string) template.JS { return template.JS(s) },
	}
	var err error
	tmpl, err = template.New("").Funcs(funcMap).ParseGlob("templates/*.tpl")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load templates: %v\n", err)
		os.Exit(1)
	}
}

func render(w http.ResponseWriter, name string, data interface{}) {
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getScheme(r *http.Request) string {
	scheme := r.URL.Scheme
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme
}

func checkHost(r *http.Request) bool {
	host := r.Host
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host == HostingWebsite && getScheme(r) == Protocol
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	protocol := getScheme(r)

	host := r.Host
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	if host == HostingWebsite {
		if protocol == Protocol {
			render(w, "index.tpl", IndexData{BaseData{Title: "DC Example Service"}})
		} else if Protocol == "https" {
			http.Redirect(w, r, "https://"+HostingWebsite, http.StatusFound)
		} else {
			http.NotFound(w, r)
		}
		return
	}

	messagetext := getMessageText(host)
	if messagetext == "" && strings.HasPrefix(host, "whd.") {
		messagetext = getMessageText(host[4:])
	}
	if messagetext == "" {
		http.NotFound(w, r)
		return
	}

	render(w, "site.tpl", SiteData{Host: host, MessageText: template.HTML(messagetext)})
}

func simpleIndexHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}
	render(w, "simple_index.tpl", BaseData{Title: "DC Example Service"})
}

func simplePostHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := r.FormValue("domain")
	subdomain := r.FormValue("subdomain")
	message := r.FormValue("message")

	if domain == "" || !isValidHostname(domain) || message == "" || !isValidMessage(message) {
		render(w, "invalid_data.tpl", BaseData{Title: "Invalid Data"})
		return
	}

	jsonData, _, errorMessage := getDomainConnectJSON(domain)
	if jsonData == nil {
		render(w, "no_domain_connect.tpl", NoDomainConnectData{BaseData: BaseData{Title: "Domain Connect Not Supported"}, Reason: errorMessage})
		return
	}

	width := jsonData.Width
	if width == 0 {
		width = 750
	}
	height := jsonData.Height
	if height == 0 {
		height = 750
	}

	checkURL1 := jsonData.URLAPI + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template1
	if !checkTemplate(checkURL1) {
		render(w, "no_domain_connect.tpl", NoDomainConnectData{BaseData: BaseData{Title: "Domain Connect Not Supported"}, Reason: "Missing template support"})
		return
	}

	dnsMessageData := dnsMessageData(message)
	qs := "domain=" + url.QueryEscape(domain) + "&RANDOMTEXT=" + url.QueryEscape(dnsMessageData) + "&IP=" + url.QueryEscape(IP)
	if subdomain != "" {
		qs = qs + "&host=" + url.QueryEscape(subdomain)
	}

	synchronousURL1 := jsonData.URLSyncUX + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template1 + "/apply?" + qs

	fqdn := domain
	if subdomain != "" {
		fqdn = subdomain + "." + fqdn
	}

	render(w, "simple_post.tpl", SimplePostData{
		BaseData:        BaseData{Title: "DC Example Service"},
		ProviderName:    jsonData.ProviderName,
		Width:           width,
		Height:          height,
		SynchronousURL1: synchronousURL1,
		FQDN:            fqdn,
	})
}

func syncIndexHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}
	render(w, "sync_index.tpl", BaseData{Title: "DC Example Service"})
}

func syncPostHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := r.FormValue("domain")
	subdomain := r.FormValue("subdomain")
	message := r.FormValue("message")

	if domain == "" || !isValidHostname(domain) || message == "" || !isValidMessage(message) {
		render(w, "invalid_data.tpl", BaseData{Title: "Invalid Data"})
		return
	}

	jsonData, txt, errorMessage := getDomainConnectJSON(domain)
	if jsonData == nil {
		render(w, "no_domain_connect.tpl", NoDomainConnectData{BaseData: BaseData{Title: "Domain Connect Not Supported"}, Reason: errorMessage})
		return
	}

	width := jsonData.Width
	if width == 0 {
		width = 750
	}
	height := jsonData.Height
	if height == 0 {
		height = 750
	}

	checkURL1 := jsonData.URLAPI + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template1
	checkURL2 := jsonData.URLAPI + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template2
	if !checkTemplate(checkURL1) || !checkTemplate(checkURL2) {
		render(w, "no_domain_connect.tpl", NoDomainConnectData{BaseData: BaseData{Title: "Domain Connect Not Supported"}, Reason: "Missing template support"})
		return
	}

	dnsMessageData := dnsMessageData(message)
	qs := "domain=" + url.QueryEscape(domain) + "&RANDOMTEXT=" + url.QueryEscape(dnsMessageData) + "&IP=" + url.QueryEscape(IP)
	if subdomain != "" {
		qs = qs + "&host=" + url.QueryEscape(subdomain)
	}

	synchronousURL1 := jsonData.URLSyncUX + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template1 + "/apply?" + qs

	sig, _ := generateSig(PrivKey, qs)
	synchronousSignedURL2 := jsonData.URLSyncUX + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template2 + "/apply?" + qs + "&sig=" + url.QueryEscape(sig) + "&key=_dck1"

	redirectURI := Protocol + "://" + HostingWebsite + "/sync_confirm?domain=" + domain + "&subdomain=" + subdomain
	qsRedirect := qs + "&redirect_uri=" + url.QueryEscape(redirectURI)

	synchronousRedirectURL1 := jsonData.URLSyncUX + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template1 + "/apply?" + qsRedirect

	sigRedirect, _ := generateSig(PrivKey, qsRedirect)
	synchronousSignedRedirectURL2 := jsonData.URLSyncUX + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template2 + "/apply?" + qsRedirect + "&sig=" + url.QueryEscape(sigRedirect) + "&key=_dck1"

	jsonBytes, _ := json.Marshal(jsonData)

	render(w, "sync_post.tpl", SyncPostData{
		BaseData:                      BaseData{Title: "Configure Domain Connect"},
		TXT:                           txt,
		JSON:                          template.JS(string(jsonBytes)),
		CheckURL1:                     checkURL1,
		CheckURL2:                     checkURL2,
		Domain:                        domain,
		ProviderName:                  jsonData.ProviderName,
		Width:                         width,
		Height:                        height,
		SynchronousURL1:               synchronousURL1,
		SynchronousSignedURL2:         synchronousSignedURL2,
		QS:                            qs,
		Sig:                           sig,
		SynchronousRedirectURL1:       synchronousRedirectURL1,
		SynchronousSignedRedirectURL2: synchronousSignedRedirectURL2,
		QSRedirect:                    qsRedirect,
		SigRedirect:                   sigRedirect,
	})
}

func syncConfirmHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	domain := r.URL.Query().Get("domain")
	subdomain := r.URL.Query().Get("subdomain")
	errorMsg := r.URL.Query().Get("error")

	if errorMsg != "" {
		render(w, "sync_error.tpl", SyncErrorData{BaseData: BaseData{Title: "Synchronous Error"}, Error: errorMsg})
		return
	}

	render(w, "sync_confirm.tpl", SyncConfirmData{BaseData: BaseData{Title: "Synchronous Configuration"}, Domain: domain, Subdomain: subdomain})
}

func asyncIndexHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}
	render(w, "async_index.tpl", BaseData{Title: "DC Example Service"})
}

func asyncPostHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := r.FormValue("domain")
	hosts := r.FormValue("hosts")

	if domain == "" || !isValidHostname(domain) {
		render(w, "invalid_data.tpl", BaseData{Title: "Invalid Data"})
		return
	}

	jsonData, txt, errorMessage := getDomainConnectJSON(domain)
	if jsonData == nil {
		render(w, "no_domain_connect.tpl", NoDomainConnectData{BaseData: BaseData{Title: "Domain Connect Not Supported"}, Reason: errorMessage})
		return
	}

	dnsProvider := jsonData.ProviderName

	checkURL1 := jsonData.URLAPI + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template1
	checkURL2 := jsonData.URLAPI + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template2
	if !checkTemplate(checkURL1) || !checkTemplate(checkURL2) {
		render(w, "no_domain_connect.tpl", NoDomainConnectData{BaseData: BaseData{Title: "Domain Connect Not Supported"}, Reason: "Missing template support"})
		return
	}

	_, hasSecret := OAuthSecrets[dnsProvider]
	apiURL, hasAPI := OAuthAPIURLs[dnsProvider]
	if !hasSecret || !hasAPI || apiURL != jsonData.URLAPI {
		render(w, "no_domain_connect.tpl", NoDomainConnectData{BaseData: BaseData{Title: "Domain Connect Not Supported"}, Reason: "Not onboarded as OAuth provider"})
		return
	}

	redirectURL := Protocol + "://" + HostingWebsite + "/async_oauth_response?domain=" + domain + "&hosts=" + hosts + "&dns_provider=" + dnsProvider

	asynchronousURL := jsonData.URLAsyncUX + "/v2/domainTemplates/providers/" + Provider + "?" +
		"domain=" + domain +
		"&host=" + hosts +
		"&client_id=" + Provider +
		"&scope=" + Template1 + "+" + Template2 +
		"&redirect_uri=" + url.QueryEscape(redirectURL)

	jsonBytes, _ := json.Marshal(jsonData)

	render(w, "async_post.tpl", AsyncPostData{
		BaseData:        BaseData{Title: "Configure Domain Connect"},
		TXT:             txt,
		JSON:            template.JS(string(jsonBytes)),
		CheckURL1:       checkURL1,
		CheckURL2:       checkURL2,
		Domain:          domain,
		Hosts:           hosts,
		ProviderName:    jsonData.ProviderName,
		AsynchronousURL: asynchronousURL,
	})
}

func asyncOAuthResponseHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	code := r.URL.Query().Get("code")
	domain := r.URL.Query().Get("domain")
	hosts := r.URL.Query().Get("hosts")
	dnsProvider := r.URL.Query().Get("dns_provider")
	errorMsg := r.URL.Query().Get("error")

	if errorMsg != "" {
		render(w, "async_error.tpl", AsyncErrorData{BaseData: BaseData{Title: "Asynchronous Error"}, Error: "Error returned from DNSProvider (" + errorMsg + ")"})
		return
	}

	redirectURL := Protocol + "://" + HostingWebsite + "/async_oauth_response?domain=" + domain + "&hosts=" + hosts + "&dns_provider=" + dnsProvider

	tokenURL := OAuthAPIURLs[dnsProvider] + "/v2/oauth/access_token?code=" + code +
		"&grant_type=authorization_code&client_id=" + Provider +
		"&client_secret=" + url.QueryEscape(OAuthSecrets[dnsProvider]) +
		"&redirect_uri=" + url.QueryEscape(redirectURL)

	resp, err := http.Post(tokenURL, "", nil)
	if err != nil || resp.StatusCode >= 300 {
		var body string
		if resp != nil {
			body = resp.Status
			resp.Body.Close()
		}
		render(w, "async_error.tpl", AsyncErrorData{BaseData: BaseData{Title: "Asynchronous Error"}, Error: "Error getting access_token: " + body})
		return
	}
	defer resp.Body.Close()

	var jsonResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jsonResponse); err != nil {
		render(w, "async_error.tpl", AsyncErrorData{BaseData: BaseData{Title: "Asynchronous Error"}, Error: "Error parsing access_token response"})
		return
	}

	accessToken, _ := jsonResponse["access_token"].(string)
	jsonBytes, _ := json.Marshal(jsonResponse)

	render(w, "async_oauth_response.tpl", AsyncOAuthResponseData{
		BaseData:     BaseData{Title: "Asynchronous Configuration"},
		Code:         code,
		URL:          tokenURL,
		Domain:       domain,
		Hosts:        hosts,
		DNSProvider:  dnsProvider,
		AccessToken:  accessToken,
		JSONResponse: template.JS(string(jsonBytes)),
	})
}

func asyncConfirmHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := r.FormValue("domain")
	subdomain := r.FormValue("subdomain")
	hosts := r.FormValue("hosts")
	message := r.FormValue("message")
	accessToken := r.FormValue("access_token")
	dnsProvider := r.FormValue("dns_provider")
	force := "0"
	if r.FormValue("force") != "" {
		force = "1"
	}

	if domain == "" || !isValidHostname(domain) || message == "" || !isValidMessage(message) || accessToken == "" || dnsProvider == "" {
		render(w, "invalid_data.tpl", BaseData{Title: "Invalid Data"})
		return
	}

	dnsMessageData := dnsMessageData(message)

	var applyURL, appliedTemplate string
	if r.FormValue("template2") != "" {
		applyURL = OAuthAPIURLs[dnsProvider] + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template2 + "/apply?domain=" + domain + "&host=" + subdomain + "&force=" + force + "&RANDOMTEXT=" + dnsMessageData + "&IP=" + IP
		appliedTemplate = "Template 2"
	} else {
		applyURL = OAuthAPIURLs[dnsProvider] + "/v2/domainTemplates/providers/" + Provider + "/services/" + Template1 + "/apply?domain=" + domain + "&host=" + subdomain + "&force=" + force + "&RANDOMTEXT=" + dnsMessageData + "&IP=" + IP
		appliedTemplate = "Template 1"
	}

	req, _ := http.NewRequest("POST", applyURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	statusCode := 0
	if err == nil && resp != nil {
		statusCode = resp.StatusCode
		resp.Body.Close()
	}

	render(w, "async_confirm.tpl", AsyncConfirmData{
		BaseData:        BaseData{Title: "Asynchronous Configuration"},
		AppliedTemplate: appliedTemplate,
		URL:             applyURL,
		Message:         message,
		AccessToken:     accessToken,
		Domain:          domain,
		Subdomain:       subdomain,
		Hosts:           hosts,
		DNSProvider:     dnsProvider,
		StatusCode:      fmt.Sprintf("%d", statusCode),
	})
}

func sigHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}
	render(w, "sig.tpl", BaseData{Title: "Signature Verification"})
}

func sigGenerateHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	qs := r.FormValue("qs")
	priv := r.FormValue("privatekey")
	if priv != "" {
		priv = strings.ReplaceAll(priv, "\\n", "")
		priv = strings.ReplaceAll(priv, "\n", "")
		priv = strings.ReplaceAll(priv, " ", "")
		priv = "-----BEGIN PRIVATE KEY-----\n" + priv + "\n-----END PRIVATE KEY-----\n"
	} else {
		priv = PrivKey
	}

	sig, _ := generateSig(priv, qs)
	render(w, "sig_generate.tpl", SigGenerateData{BaseData: BaseData{Title: "Signature Verification"}, Sig: sig})
}

func sigFetchHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := r.FormValue("domain")
	key := r.FormValue("key")

	pubKey, recordStrings, err := getPublicKey(key + "." + domain)
	if err != nil {
		pubKey = ""
		recordStrings = nil
	} else {
		pubKey = "-----BEGIN PUBLIC KEY-----\n" + pubKey + "\n-----END PUBLIC KEY-----\n"
	}

	render(w, "sig_fetch.tpl", SigFetchData{
		BaseData:      BaseData{Title: "Signature Public Key Fetch"},
		Domain:        domain,
		Key:           key,
		PubKey:        pubKey,
		RecordStrings: recordStrings,
	})
}

func sigVerifyHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := r.FormValue("domain")
	key := r.FormValue("key")
	pub := r.FormValue("publickey")
	if pub != "" {
		pub = strings.ReplaceAll(pub, "\\n", "")
		pub = strings.ReplaceAll(pub, " ", "")
		pub = "-----BEGIN PUBLIC KEY-----\n" + pub + "\n-----END PUBLIC KEY-----\n"
	}
	sig := r.FormValue("sig")
	qs := r.FormValue("qs")

	var recordStrings []string
	if pub == "" {
		var err error
		pub, recordStrings, err = getPublicKey(key + "." + domain)
		if err != nil {
			pub = ""
			recordStrings = nil
		} else {
			pub = "-----BEGIN PUBLIC KEY-----\n" + pub + "\n-----END PUBLIC KEY-----\n"
		}
	}

	verified := verifySig(pub, sig, qs)

	render(w, "sig_verify.tpl", SigVerifyData{
		BaseData:      BaseData{Title: "Signature Verification"},
		Domain:        domain,
		Key:           key,
		Sig:           sig,
		QS:            qs,
		Verified:      verified,
		PubKey:        pub,
		RecordStrings: recordStrings,
	})
}

func sigVerifyURLHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	urlStr := r.FormValue("url")
	domain := r.FormValue("domain")

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	params := strings.Split(parsedURL.RawQuery, "&")
	var sig, key, qs string
	for _, param := range params {
		if strings.HasPrefix(param, "sig=") {
			sig, _ = url.QueryUnescape(param[4:])
		} else if strings.HasPrefix(param, "key=") {
			key, _ = url.QueryUnescape(param[4:])
		} else {
			if qs == "" {
				qs = param
			} else {
				qs = qs + "&" + param
			}
		}
	}

	pub, recordStrings, err := getPublicKey(key + "." + domain)
	if err != nil {
		pub = ""
		recordStrings = nil
	} else {
		pub = "-----BEGIN PUBLIC KEY-----\n" + pub + "\n-----END PUBLIC KEY-----\n"
	}

	verified := verifySig(pub, sig, qs)

	render(w, "sig_verify.tpl", SigVerifyData{
		BaseData:      BaseData{Title: "Signature Verification"},
		Domain:        domain,
		Key:           key,
		Sig:           sig,
		QS:            qs,
		Verified:      verified,
		PubKey:        pub,
		RecordStrings: recordStrings,
	})
}

func clientHandler(w http.ResponseWriter, r *http.Request) {
	if !checkHost(r) {
		http.NotFound(w, r)
		return
	}
	render(w, "client.tpl", BaseData{Title: "Client Side"})
}

func ddnsCodeHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	if host != DynamicDNSWebsite || getScheme(r) != Protocol {
		http.NotFound(w, r)
		return
	}

	code := r.URL.Query().Get("code")
	errorMsg := r.URL.Query().Get("error")

	if errorMsg != "" {
		render(w, "ddns_error.tpl", DDNSErrorData{BaseData: BaseData{Title: "Domain Connect DDNS Error"}, Error: errorMsg})
		return
	}

	render(w, "ddns_oauth_code.tpl", DDNSCodeData{BaseData: BaseData{Title: "Domain Connect Dynamic DNS"}, OAuthCode: code})
}

func staticHandler(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP(w, r)
}
