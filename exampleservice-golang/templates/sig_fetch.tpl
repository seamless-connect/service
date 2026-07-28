{{template "header" .}}

<h1>Signature Public Key Fetch</h1>

<h3>Domain</h3>
{{.Domain}}

<h3>Key</h3>
{{.Key}}

<h3>Public Key DNS Records</h3>
{{if .RecordStrings}}
{{range .RecordStrings}}
{{.}}
{{end}}
<p/>
{{end}}

<h3>Public Key</h3>
{{.PubKey}}

{{template "footer" .}}
