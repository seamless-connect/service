package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

func verifySig(publicKey, signature, data string) bool {
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return false
	}
	pk, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false
	}
	rsaPub, ok := pk.(*rsa.PublicKey)
	if !ok {
		return false
	}
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	hash := sha256.Sum256([]byte(data))
	err = rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hash[:], sigBytes)
	return err == nil
}

func generateSig(privateKey, data string) (string, error) {
	block, _ := pem.Decode([]byte(privateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}
	pk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try parsing as generic private key
		key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return "", err
		}
		var ok bool
		pk, ok = key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("not an RSA private key")
		}
	}
	hash := sha256.Sum256([]byte(data))
	sig, err := rsa.SignPKCS1v15(rand.Reader, pk, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func getPublicKey(domain string) (string, []string, error) {
	records, err := net.LookupTXT(domain)
	if err != nil {
		return "", nil, err
	}

	segments := make(map[int]string)
	var recordStrings []string
	for _, text := range records {
		recordStrings = append(recordStrings, text)
		parts := strings.Split(text, ",")
		index := -1
		indexData := ""
		for _, kv := range parts {
			kv = strings.TrimSpace(kv)
			if strings.HasPrefix(kv, "p=") {
				idx, err := strconv.Atoi(kv[2:])
				if err == nil {
					index = idx
				}
			} else if strings.HasPrefix(kv, "d=") {
				indexData = kv[2:]
			} else if strings.HasPrefix(kv, "a=") && kv != "a=RS256" {
				return "", nil, fmt.Errorf("unsupported algorithm")
			} else if strings.HasPrefix(kv, "t=") && kv != "t=x509" {
				return "", nil, fmt.Errorf("unsupported key type")
			}
		}
		if index != -1 && indexData != "" {
			segments[index] = indexData
		}
	}

	var keys []int
	for k := range segments {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var pembits string
	for _, k := range keys {
		pembits += strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(segments[k], "\n", ""), "\\n", ""))
	}

	return pembits, recordStrings, nil
}
