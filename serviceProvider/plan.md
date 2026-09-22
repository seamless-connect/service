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
  * Secret Key interface for signing
    * Assume the interface is backed by a path, hashicorp vault
    * The secret source needs to be reusable across templates
  * Template variable fill definitions
* Templates that have undefined varibles need to cause Internal Server Error
  when attempted to use, with error telling there is a mismatch
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

During the processing, if anything fails entire request fails.

## Input

These are the dc-api inputs:

* Domain input field
* Template selector
* groupId selector, if needed once template is selected

Input validation:

* The domain is checked to be a valid DNS name
* Template is confirmed to be in use for this Service Provider
* Choose sync / async (if Service Provider allows choosing)

### Later implementation: input series

* When a single template apply is fully implemented the input is changed to
  * Support multiple groupId apply series in pre-defined sequence
  * And once that works multiple templates across DNS providers is made to
    work
* Note. Multi-apply must be fully known from start of the request. There is
  no plans of converting this service to programmable 'if-fails-then-apply'
  system or service.  That means after the input is parsed _every_ apply in
  the series is known at that moment, including information which of the
  changes can be applied concurrently.

## DNS Provider search

* Get _domainconnect.<domain> TXT record
   * Cache response as long TTL allows
* Get v2/<domain>/settings
   * Parse DNS Provider definition
   * Cache response according to http cache-control
* Check the requested sync / async flow is supported

## Apply url construction

Construct /apply URL for the user

* The URL path defines the template, and query string contains all the required %variables%
  and validation details
* Remember to take account groupId
* Generate request identifier
  * User should be made aware of this identifier

## Authentication and consent

* Require consent
* Either oauth or send user to DNS provider apply implementation

### Later authentication and consent work

* The authentication and authorization will get a lot more complicated when
  multiple updates are coordinated potentially across DNS providers

## Apply the changes

* ditto

## Report status

* Let user know if change was successful
* Which records were added, modified, deleted
* Regardless of success or failure include request identifier in response

Note.  Eventually the apply needs to become a batch job, and status turns
into a work report that one needs to fetch using request id.
