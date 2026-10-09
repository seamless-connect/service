package internal

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// maxDomainLength is the length of a DNS name in its presentation form,
	// without the root label.
	maxDomainLength = 253
	// maxLabelLength is the length of a single label of a DNS name.
	maxLabelLength = 63
)

// normalizeDomain checks that the domain is a DNS name that a search can be
// made for, and returns it in the lower case form that DNS names are compared
// in.  Surrounding space and a trailing dot are dropped, so that a fully
// qualified name is accepted as well.
//
// The result is used as part of a DNS query name, which is why every label is
// checked rather than left to the resolver.
func normalizeDomain(domain string) (string, error) {
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if name == "" {
		return "", errors.New("no domain was given")
	}
	if len(name) > maxDomainLength {
		return "", fmt.Errorf("a domain is at most %d characters long", maxDomainLength)
	}
	// A domain has at least the label of the name and the label of the
	// zone it is in.
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("%q is not a domain name", domain)
	}
	for _, label := range labels {
		if err := validLabel(label); err != nil {
			return "", fmt.Errorf("%q is not a valid domain name: %w", domain, err)
		}
	}
	return name, nil
}

// validLabel checks that a label of a domain name consists of letters, digits,
// and dashes, and that it does not start or end with a dash.  Upper case
// letters are not accepted, because normalizeDomain has already lowered them.
func validLabel(label string) error {
	if len(label) == 0 {
		return errors.New("a label of a domain name is empty")
	}
	if len(label) > maxLabelLength {
		return fmt.Errorf("the label %q is longer than %d characters", label, maxLabelLength)
	}
	if label[0] == '-' {
		return fmt.Errorf("the label %q starts with a dash", label)
	}
	if label[len(label)-1] == '-' {
		return fmt.Errorf("the label %q ends with a dash", label)
	}
	for i := 0; i < len(label); i++ {
		char := label[i]
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '-':
		default:
			return fmt.Errorf("the label %q has the character %q, which is not a letter, a digit, or a dash", label, string(char))
		}
	}
	return nil
}
