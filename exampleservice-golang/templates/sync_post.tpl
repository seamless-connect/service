{{template "header" .}}

<h2>Discovering the DNS Provider<h2>

<h4>Step 1</h4>
Query for the _domainconnect TXT record at {{.Domain}}:
<p/>
<p style="font-family:courier; word-break:break-all">{{.TXT}}</p>

<h4>Step 2</h4>
Calling URL https://{{.TXT}}/v2/{{.Domain}}/settings:
<p/>
<pre style="font-family:Courier" id="json"></pre>
<script>document.getElementById("json").innerHTML = JSON.stringify({{.JSON}}, undefined, 2);</script>

<h4>Step 3</h4>
URLs to query for support of templates:
<p/>
<p style="font-family:courier; word-break:break-all">{{.CheckURL1}}</p>
<p/>
<p style="font-family:courier; word-break:break-all">{{.CheckURL2}}</p>

<h4>Step 4</h4>
URL to call domain connect for template 1

<pre style="font-family:courier; word-break:break-all">
{{.SynchronousURL1}}
</pre>

<h2>Next Step</h2>
<h4>New Tab</h4>
<a target=_new href='{{.SynchronousURL1}}'>Configure Template 1</a>
<p/>
<a target=_new href='{{.SynchronousSignedURL2}}'>Configure Template 2 with Signature Verification</a>
<form method="post" action="sig_verify">
<input name="domain" type="hidden" value="exampleservice.domainconnect.org">
<input name="key" type="hidden" value="_dck1">
<input name="sig" type="hidden" value="{{.Sig}}">
<input name="qs" type="hidden" value="{{.QS}}">
<input type="submit" value="Verify Signature" />
</form>

<h4>New Window</h4>
<a target=_new href='javascript:null(void);' onclick='window.open("{{.SynchronousURL1}}", "", "width={{.Width}},height={{.Height}}");'>Configure Template 1</a>
<p/>
<a target=_new href='javascript:null(void);' onclick='window.open("{{.SynchronousSignedURL2}}", "", "width={{.Width}},height={{.Height}}");'>Configure Template 2 with Signature Verification</a>
<form method="post" action="sig_verify">
<input name="domain" type="hidden" value="exampleservice.domainconnect.org">
<input name="key" type="hidden" value="_dck1">
<input name="sig" type="hidden" value="{{.Sig}}">
<input name="qs" type="hidden" value="{{.QS}}">
<input type="submit" value="Verify Signature" />
</form>

<h4>In Place w/ Redirect</h4>
<a href='{{.SynchronousRedirectURL1}}'>Configure Template 1</a>
<p/>
<a href='{{.SynchronousSignedRedirectURL2}}'>Configure Template 2 with Signature Verification</a>
<form method="post" action="sig_verify">
<input name="domain" type="hidden" value="exampleservice.domainconnect.org">
<input name="key" type="hidden" value="_dck1">
<input name="sig" type="hidden" value="{{.SigRedirect}}">
<input name="qs" type="hidden" value="{{.QSRedirect}}">
<input type="submit" value="Verify Signature" />
</form>

{{template "footer" .}}
