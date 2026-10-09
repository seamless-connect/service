package internal

import (
	_ "embed"
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/seamlessdns/service/providerFormats"

	"github.com/rs/zerolog/log"
)

// The fragments of the input page share one template set, because the fragment
// of the DNS Provider search carries the template selection, which is a
// fragment of its own.
//
//go:embed connectPage.html
var connectPageHTML string

//go:embed dnsProviderPage.html
var dnsProviderPageHTML string

//go:embed templateApply.html
var templateApplyHTML string

//go:embed templateVariables.html
var templateVariablesHTML string

//go:embed curlRequest.html
var curlRequestHTML string

var pagesTemplate = template.Must(template.New("pages").Parse(
	connectPageHTML + dnsProviderPageHTML + templateApplyHTML +
		templateVariablesHTML + curlRequestHTML))

// variablePattern matches one %variable% string.  The name is whatever sits
// between the percent signs and holds neither a percent sign nor space.
var variablePattern = regexp.MustCompile(`%[^%\s]+%`)

// templateChoice is one template that can be chosen on the page.
type templateChoice struct {
	// ID is the key the template is configured under.  It is what the page
	// sends back, so that a template that is configured twice under two
	// keys is still told apart.
	ID string
	// Description tells what is being selected, "providerId.serviceId".
	Description string
}

// templateVariable is one %variable% string that has to be given a value.
type templateVariable struct {
	Name string
}

// templateInput is what the page needs to show the selection of a template and
// to collect the values of the records of the chosen template.
type templateInput struct {
	// Domain is the domain whose DNS Provider the templates are checked
	// against.
	Domain string
	// Choices are the templates that can be chosen.
	Choices []templateChoice
	// Selected is the key of the template that is chosen.
	Selected string
	// Description tells which template is chosen, "providerId.serviceId".
	Description string
	// Supported is what the DNS Provider has of the chosen template.
	Supported providerFormats.SupportedTemplate
	// Grouped tells that the records of the template are grouped, so that a
	// group has to be chosen before the variables of it are collected.
	Grouped bool
	// Groups are the group ids of the template, in the order the records
	// use them.
	Groups []string
	// Group is the group id that is chosen.
	Group string
	// Variables are the %variable% strings that have to be defined.
	Variables []templateVariable
	// Warning tells what is worth knowing about the choice even though the
	// choice can be used.
	Warning string
	// Error tells why the template cannot be used, if it cannot.
	Error string
}

// templateSelect collects the template that is chosen, and with it the group
// and the %variable% strings that have to be filled in.  It is asked again on
// every change of the selection, so that what the page collects always belongs
// to the template and the group that are chosen at that moment.
func templateSelect(c *gin.Context, tmpls Templates) {
	input := templateSelection(c.Query("domain"), tmpls)

	// Nothing has been chosen yet, so the section that follows the selection
	// has nothing of its own to show.
	if id := c.Query("template"); id != "" {
		conf, ok := tmpls[id]
		if !ok {
			input.Error = fmt.Sprintf("template %q is not configured", id)
			renderFragment(c, pagesTemplate.Lookup("templateVariables"), input)
			return
		}
		input.Selected = id
		input.Description = conf.Template.ProviderID + "." + conf.Template.ServiceID

		// The DNS Provider has to say that it supports the template before
		// anything is collected for it.  Applying a template that the DNS
		// Provider does not support would fail there, and the user of the
		// page is better served by being told here.
		definition, ok := definitions.get(input.Domain)
		if !ok {
			input.Error = fmt.Sprintf(
				"what the DNS Provider of %s supports is not known, because its settings could not be read, so it cannot be told whether %s is supported",
				input.Domain, input.Description)
			renderFragment(c, pagesTemplate.Lookup("templateVariables"), input)
			return
		}
		supported, err := readSupportedTemplate(c.Request.Context(), definition.Provider, conf.Template)
		if err != nil {
			log.Warn().Err(err).Str("domain", input.Domain).Str("template", input.Description).
				Msg("DNS Provider cannot be asked about the template")
			input.Error = err.Error()
			renderFragment(c, pagesTemplate.Lookup("templateVariables"), input)
			return
		}
		input.Supported = supported
		// The template can be applied with a version that the DNS Provider
		// does not have, but that is worth saying out loud: the records that
		// are applied may not be the ones the DNS Provider expects.
		if supported.Version != conf.Template.Version {
			log.Warn().Str("domain", input.Domain).Str("template", input.Description).
				Uint("supported", uint(supported.Version)).
				Uint("configured", uint(conf.Template.Version)).
				Msg("DNS Provider has another version of the template")
			input.Warning = fmt.Sprintf(
				"the DNS Provider has %s in version %d, while this Service Provider has it in version %d",
				input.Description, supported.Version, conf.Template.Version)
		}

		input.Groups = templateGroups(conf.Template)
		input.Grouped = len(input.Groups) > 0

		group := c.Query("group")
		if input.Grouped && group != "" && !slices.Contains(input.Groups, group) {
			input.Error = fmt.Sprintf("group %q is not part of template %q", group, input.Description)
			renderFragment(c, pagesTemplate.Lookup("templateVariables"), input)
			return
		}
		input.Group = group
		input.Variables = templateVariables(conf.Template, group)
	}
	// The template selector asks for this fragment, so that it carries the
	// empty element its own change replaces.
	renderFragment(c, pagesTemplate.Lookup("templateVariables"), input)
}

// templateSelection lists the templates that can be chosen on the page.
func templateSelection(domain string, tmpls Templates) templateInput {
	input := templateInput{Domain: domain}
	for id, conf := range tmpls {
		// A template that could not be read has no provider to describe it
		// with, so it cannot be chosen.
		if conf.Template.ProviderID == "" || conf.Template.ServiceID == "" {
			log.Warn().Str("id", id).Msg("template is not loaded, it cannot be chosen")
			continue
		}
		input.Choices = append(input.Choices, templateChoice{
			ID:          id,
			Description: conf.Template.ProviderID + "." + conf.Template.ServiceID,
		})
	}
	// The configuration is a map, which has no order of its own, and the
	// choices on the page must not move around between requests.
	sort.Slice(input.Choices, func(i, j int) bool {
		if input.Choices[i].Description != input.Choices[j].Description {
			return input.Choices[i].Description < input.Choices[j].Description
		}
		return input.Choices[i].ID < input.Choices[j].ID
	})
	return input
}

// templateGroups returns the group ids that the records of a template use, in
// the order the records use them.  The records of a template that does not
// group them have no group at all.
func templateGroups(tmpl providerFormats.Template) []string {
	var groups []string
	for _, record := range tmpl.Records {
		if record.GroupID != "" && !slices.Contains(groups, record.GroupID) {
			groups = append(groups, record.GroupID)
		}
	}
	return groups
}

// templateVariables returns the %variable% strings of the records that belong
// to group, in the order the records hold them.  An empty group collects the
// variables of every record, which is what a template that does not group its
// records needs.
func templateVariables(tmpl providerFormats.Template, group string) []templateVariable {
	grouped := len(templateGroups(tmpl)) > 0
	// Which group is applied has not been chosen yet.
	if grouped && group == "" {
		return nil
	}
	var variables []templateVariable
	seen := make(map[string]bool)
	for _, record := range tmpl.Records {
		if grouped && record.GroupID != group {
			continue
		}
		for _, name := range recordVariables(record) {
			if seen[name] {
				continue
			}
			seen[name] = true
			variables = append(variables, templateVariable{Name: name})
		}
	}
	return variables
}

// recordVariables returns the %variable% strings of one record, in the order
// they appear in it, with the repeats left in.
func recordVariables(record providerFormats.Record) []string {
	// Only the fields that are free text can hold a variable.  The others
	// are one of a fixed set of values.
	fields := []string{
		record.Host, record.Name, record.PointsTo, record.Data,
		record.Protocol, record.Service, record.Target,
		record.SPFRules, record.TxtCMP,
		string(record.TTL), string(record.Priority),
		string(record.Weight), string(record.Port),
	}
	var names []string
	for _, field := range fields {
		for _, match := range variablePattern.FindAllString(field, -1) {
			// The percent signs are part of the template notation, and
			// the value is what the name stands for.
			names = append(names, match[1:len(match)-1])
		}
	}
	return names
}
