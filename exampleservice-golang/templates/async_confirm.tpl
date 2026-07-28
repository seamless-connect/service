{{template "header" .}}

<h2>Async Confirm</h2>
A call to apply the template was made to:
<p/>
<div style="font-family:courier;word-break:no-break">{{.URL}}</div>
<p/>
A status of '{{.StatusCode}}' was returned.
<p/>
{{if eq .StatusCode "200"}}
  You've applied '{{.AppliedTemplate}}' to '{{.Domain}}'
  {{if and (ne .Subdomain "") (ne .Subdomain "None")}}
      with sub domain '{{.Subdomain}}'
  {{end}}
  with the message '{{.Message}}'. You can apply Template 1 or Template 2 to any of the authorized domain/sub-domains.
{{end}}

<h2>Interesting information (read only):</h2>
<table border=1 cellpadding=3>
<tr><td>Domain:</td><td>{{.Domain}}</td></tr>
<tr><td>Hosts:</td><td>{{.Hosts}}</td></tr>
<tr><td>DNS Provider:</td><td>{{.DNSProvider}}</td></tr>
<tr><td valign="top">Access&nbsp;Token:</td><td><div style="font-family:courier;word-break:no-break">{{.AccessToken}}</td></tr>
</table>
<form method="post" action="/async_confirm">

<h2>Apply template:</h2>
<input name=domain type=hidden value="{{.Domain}}"/>
<input name=hosts type=hidden value="{{.Hosts}}"/>
<input name=dns_provider type=hidden value="{{.DNSProvider}}"/>
<input name=access_token type=hidden value="{{.AccessToken}}"/>
<table>
<tr><td>Domain:</td><td>{{.Domain}}</td></tr>
<tr><td>Hosts:</td><td>{{.Hosts}}</td></tr>
<tr><td>---</td><td></td></tr>
<tr><td>Message:</td><td><input size=50 name=message type=text value=""/></td></tr>
<tr><td>---</td><td></td></tr>
<tr><td>Sub Domain:</td><td><input size=50 name=subdomain type=text value=""/></td></tr>
<tr><td>Force:</td><td><input type="checkbox" name="force" value="1"/></td></tr>
<tr><td>&nbsp;</td><td><input type="submit" value="Apply Template 1"/></td>
<tr><td>&nbsp;</td><td><input type="submit" value="Apply Template 2" name="template2"/></td>
</table>
</form>
{{template "footer" .}}
