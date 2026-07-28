{{template "header" .}}

<h1>Signature Verification Test</h1>

{{if .Verified}}
<h1 style="color:green">Passed</h1>
{{else}}
<h1 style="color:red">Failed</h1>
{{end}}

<h3>Domain</h3>
{{.Domain}}

<h3>Key</h3>
{{.Key}}

<h3>Sig</h3>
{{.Sig}}

<h3>QS</h3>
{{.QS}}

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
