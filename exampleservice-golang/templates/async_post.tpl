{{template "header" .}}

<h2>Discovering the DNS Provider<h2>

<h4>Step 1</h4>
Query for the _domainconnect TXT record at {{.Domain}}:
<p/>
<p style="font-family:courier; word-break:break-all">{{.TXT}}</p>

<h4>Step 2</h4>
json returned by https://{{.TXT}}/v2/{{.Domain}}/settings:
<p/>
<pre style="font-family:Courier" id="json"></pre>
<script>document.getElementById("json").innerHTML = JSON.stringify({{.JSON}}, undefined, 2);</script>

<h4>Step 3</h4>
URLs to query for support of templates:
<p/>
<p style="font-family:courier; word-break:break-all">{{.CheckURL1}}</p>
<p/>
<p style="font-family:courier; word-break:break-all">{{.CheckURL2}}</p>

<h4>Step 4<h4>
URL to call and start oAuth process

<pre style=font-family:courier; word-break:break-all">
{{.AsynchronousURL}}
</pre>

<h2>Next Step</h2>


<p style="font-family:courier; word-break:break-all">{{.AsynchronousURL}}</p>
<p/>
Provider Found: Your domain uses {{.ProviderName}}.
<p/>
<a href='{{.AsynchronousURL}}'>Get Asynchronous Permission</a>

{{template "footer" .}}
