{{template "header" .}}

<h2>Provider Found: Your domain uses {{.ProviderName}}</h2>

Your provider is {{.ProviderName}}.

<h3>Step 1</h3>
<a target=_new href='javascript:null(void);' onclick='window.open("{{.SynchronousURL1}}", "", "width={{.Width}},height={{.Height}}");'>Click here for one easy setup.</a>

<h3>Step 2</h3>
<a href='http://{{.FQDN}}'>Visit your site</a>

<p/>
Note: It may take a few minutes for your configuration to take affect.

{{template "footer" .}}
