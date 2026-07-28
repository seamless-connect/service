{{template "header" .}}

<h1>Applied</h1>
You've applied a template to domain '{{.Domain}}'
{{if .Subdomain}}
  and subdomain '{{.Subdomain}}'
{{end}}

{{template "footer" .}}
