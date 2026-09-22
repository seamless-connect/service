package providerFormats

import (
	"errors"
)

type DNSProvider struct {
	ProviderID      string   `json:"providerId" validate:"required,min=1,max=64"`
	ProviderName    string   `json:"providerName" validate:"required,min=1,max=64"`
	ProviderDN      string   `json:"providerDisplayName,omitempty" validate:"omitempty,max=64"`
	URLSyncUX       string   `json:"urlSyncUX,omitempty" validate:"omitempty,http_url"`
	URLapi          string   `json:"urlAPI" validate:"http_url"`
	Nameservers     []string `json:"nameServers,omitempty"`
	URLAsyncUX      Reserved `json:"urlAsyncUX,omitempty"`
	Width           Reserved `json:"width,omitempty"`
	Height          Reserved `json:"height,omitempty"`
	URLControlPanel Reserved `json:"urlControlPanel,omitempty"`
}

type Reserved string

var ErrReserved = errors.New("use of reserved value")

func (_ *Reserved) UnmarshalJSON(_ []byte) error {
	return ErrReserved
}

func (_ *Reserved) MarshalJSON(_ []byte) error {
	return nil
}

type SupportedTemplate struct {
	Version TmplVersion `json:"version"`
}
