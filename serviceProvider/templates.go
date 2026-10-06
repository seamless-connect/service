package serviceProvider

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/seamlessdns/service/serviceProvider/internal"

	"github.com/rs/zerolog/log"
)

func readTemplates(tmpls internal.Templates) {
	for id, conf := range tmpls {
		log.Debug().Str("id", id).Msg("reading template")
		u, err := url.Parse(conf.Source)
		if err != nil {
			log.Error().Err(err).Str("id", id).Msg("cannot parse url")
			continue
		}
		var tmplBytes []byte

		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			tmplBytes, err = getURL(conf.Source)
			if err != nil {
				log.Error().Err(err).Str("id", id).Msg("template url failed")
				continue
			}
		case "file":
			log.Debug().Str("file", u.Path).Msg("getting template from file")
			tmplBytes, err = os.ReadFile(u.Path)
			if err != nil {
				log.Error().Err(err).Str("id", id).Msg("template file failed")
				continue
			}
		default:
			log.Error().Str("id", id).Msg("unknown template url schema")
			continue
		}
		err = json.Unmarshal(tmplBytes, &conf.Template)
		if err != nil {
			log.Error().Err(err).Str("id", id).Msg("template json failed")
			continue
		}
		conf.SigningKey = getPrivateKey(conf.SecretKey)
	}
}

func getURL(url string) ([]byte, error) {
	log.Debug().Str("url", url).Msg("getting template from url")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Warn().Str("url", url).Err(err).Msg("could not close http body")
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bodyBytes, nil
}

func getPrivateKey(pathToKey string) any {
	log.Debug().Msg("read private key")
	keyBytes, err := os.ReadFile(pathToKey)
	if err != nil {
		log.Error().Err(err).Str("path", pathToKey).Msg("could not read file")
		return nil
	}
	keyBlock, _ := pem.Decode(keyBytes)
	if keyBlock == nil {
		log.Error().Err(err).Str("path", pathToKey).Msg("could not decode private key")
		return nil
	}
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		log.Error().Err(err).Str("path", pathToKey).Msg("could not parse private key")
		return nil
	}
	return key
}
