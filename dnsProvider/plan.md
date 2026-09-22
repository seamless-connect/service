# Introduction

This is a web service.

# Startup

* Parse command-line options, that tell where the config file is
  * Everything else is defined in config, that is in toml format (not json
    because comments must be supported)
* Usual service start stuff, such as start listening socket
* Structured logging
* Prometheus metrics
* OpenTelemetry setup

## Template Config

* Template definitions, that have for each template
  * Path or URL to template
    * If URL cache control definition is required
* DNSProvider spec aka v2/{domain}/settings response
  * Must be reusable definition
  * Use domain and host name as possible DNSProvider data selector
  * Different hosts can provide different answers
* A config validator is required for ease of use
* Service Provider service discovery
  * Assume Service Provider is running multiple instances, that need to
    gossip state of the template up to dateness

# Template keeper

* This component is also used in DNS Provider code
* Runs immediately after startup
* Keeps templates up to date
  * If templates are on local file use inotify
  * If templates are collected from URLs periodic updates using cache
    control
    * Cache recheck interval (default to 4 hours)
    * Recheck jitter (default to 15 mins)
* If / when updates are found gossip to neighbouring instances,
  and make instances to copy the update from each other (rather than
  internet)
* The template keeper will convert template from json to a struct
  * During the conversion notes are taken about %variables% that can be
    groupId scoped

# Requests

During the request processing, if anything fails entire request fails.

## Inputs

* DNS provier will receive requests initiated by Service Provider, the
  following URIs must exist
  * v2/{domain}/settings
    * Will return DNSProvider struct
  * v2/domainTemplates/providers/{providerId}/services/{serviceId}
    * Will return template version
  * v2/domainTemplates/providers/{providerId}/services/{serviceId}/apply
    * Will make the DNS change, and report status

Input validation

* All and each request always checks the requested template is supported
* The apply is complicated, see later text

# Apply requests

* FIXME following text is not well considered
* Collect url parameters
* Cross ref the template variable requirements (noting groupId) can be
  fulfilled
* Validate the signature
* Authenticate and authorize
* Convert the template to DNS updates
  * In case of ddns this will be a 'update add/delete resource record'
  * Or when DNS vendor series of their api urls
* Respond to user how things went
