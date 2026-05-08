package newvm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) GetZone(ctx context.Context, zoneName string) (*ZoneWrapper, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/backend/com.newvm.network/v1/zone/%s", c.HostURL, url.PathEscape(zoneName)),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	var response ZoneWrapper
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if !response.Success {
		return nil, fmt.Errorf("api returned success=false while reading zone %q", zoneName)
	}

	return &response, nil
}

func (c *Client) CreateDnsRecord(ctx context.Context, zoneName string, payload DnsRecordCreateRequest) (*DnsRecordMutationResponse, error) {
	return c.mutateDnsRecord(ctx, http.MethodPost, zoneName, payload)
}

func (c *Client) UpdateDnsRecord(ctx context.Context, zoneName string, payload DnsRecordUpdateRequest) (*DnsRecordMutationResponse, error) {
	return c.mutateDnsRecord(ctx, http.MethodPut, zoneName, payload)
}

func (c *Client) DeleteDnsRecord(ctx context.Context, zoneName string, hash string) (*DnsRecordMutationResponse, error) {
	payload := DnsRecordDeleteRequest{
		Hash: hash,
	}

	return c.mutateDnsRecord(ctx, http.MethodDelete, zoneName, payload)
}

func (c *Client) mutateDnsRecord(ctx context.Context, method, zoneName string, payload any) (*DnsRecordMutationResponse, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		fmt.Sprintf("%s/backend/com.newvm.network/v1/zone/%s", c.HostURL, url.PathEscape(zoneName)),
		bytes.NewReader(raw),
	)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	var response DnsRecordMutationResponse
	if len(body) == 0 {
		return &response, nil
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if !response.Success {
		return nil, fmt.Errorf("api returned success=false for %s on zone %q", method, zoneName)
	}

	return &response, nil
}

func FindDnsRecordByHash(records []DnsRecord, hash string) *DnsRecord {
	for i := range records {
		if records[i].Hash == hash {
			return &records[i]
		}
	}

	return nil
}

func DnsRecordMatches(record DnsRecord, want DnsRecord) bool {
	if !strings.EqualFold(record.Type, want.Type) {
		return false
	}
	if record.Name != want.Name {
		return false
	}
	if record.TTL != want.TTL {
		return false
	}

	recordType := strings.ToUpper(strings.TrimSpace(record.Type))

	switch recordType {
	case "CAA":
		if record.Flag != want.Flag {
			return false
		}
		if record.Tag != want.Tag {
			return false
		}

		recordValue := record.Value
		if recordValue == "" {
			recordValue = record.Content
		}

		if normalizeDnsValue(recordValue) != normalizeDnsValue(want.Value) {
			return false
		}

	case "CNAME":
		recordValue := record.Target
		if recordValue == "" {
			recordValue = record.Content
		}

		wantValue := want.Target
		if wantValue == "" {
			if want.Value != "" {
				wantValue = want.Value
			} else {
				wantValue = want.Content
			}
		}

		if normalizeDnsValue(recordValue) != normalizeDnsValue(wantValue) {
			return false
		}

	case "PTR":
		recordValue := record.Content
		if recordValue == "" {
			recordValue = record.Value
		}

		wantValue := want.Content
		if wantValue == "" {
			wantValue = want.Value
		}

		if normalizeDnsValue(recordValue) != normalizeDnsValue(wantValue) {
			return false
		}

	default:
		if normalizeDnsValue(record.Content) != normalizeDnsValue(want.Content) {
			return false
		}
	}

	if record.Priority != want.Priority {
		return false
	}
	if record.Weight != want.Weight {
		return false
	}
	if record.Port != want.Port {
		return false
	}

	// target alleen vergelijken voor types waar het echt semantisch apart relevant is
	if recordType != "CNAME" {
		if normalizeDnsValue(record.Target) != normalizeDnsValue(want.Target) {
			return false
		}
	}

	return true
}

func FindDnsRecord(records []DnsRecord, want DnsRecord) *DnsRecord {
	for i := range records {
		if DnsRecordMatches(records[i], want) {
			return &records[i]
		}
	}

	return nil
}

func normalizeDnsValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.TrimSuffix(value, ".")
	return value
}

func NormalizeDnsRecordValue(recordType, value string) string {
	switch strings.ToUpper(strings.TrimSpace(recordType)) {
	case "A", "AAAA", "CAA", "CNAME", "PTR", "TXT":
		return normalizeDnsValue(value)
	default:
		return strings.TrimSpace(value)
	}
}

func NormalizeDnsRecordName(zone, name string) string {
	name = strings.TrimSpace(strings.TrimSuffix(name, "."))
	zone = strings.TrimSpace(strings.TrimSuffix(zone, "."))

	if name == zone {
		return ""
	}

	suffix := "." + zone
	if strings.HasSuffix(name, suffix) {
		return strings.TrimSuffix(name, suffix)
	}

	return name
}

func NormalizeDnsRecordZoneAndName(recordType, zone, name string) (string, string, error) {
	recordType = strings.ToUpper(strings.TrimSpace(recordType))

	if recordType != "PTR" {
		return NormalizeDnsRecordName(zone, name), zone, nil
	}

	ip := net.ParseIP(strings.TrimSpace(zone))
	if ip == nil {
		// Already a reverse DNS zone.
		return strings.TrimSpace(name), strings.TrimSpace(zone), nil
	}

	if ipv4 := ip.To4(); ipv4 != nil {
		return fmt.Sprintf("%d", ipv4[3]),
			fmt.Sprintf("%d.%d.%d.in-addr.arpa", ipv4[2], ipv4[1], ipv4[0]),
			nil
	}

	ip16 := ip.To16()
	if ip16 == nil {
		return "", "", fmt.Errorf("invalid IPv6 address %q", zone)
	}

	hex := fmt.Sprintf("%032x", ip16)

	nibbles := make([]string, 0, 32)
	for i := len(hex) - 1; i >= 0; i-- {
		nibbles = append(nibbles, string(hex[i]))
	}

	// Default to /64 reverse zone: first 16 reversed nibbles become the record name,
	// remaining 16 reversed nibbles become the ip6.arpa zone.
	recordName := strings.Join(nibbles[:16], ".")
	reverseZone := strings.Join(nibbles[16:], ".") + ".ip6.arpa"

	return recordName, reverseZone, nil
}

func (c *Client) UpdateAddressPtr(ctx context.Context, ipAddress string, fqdn *string) (*AddressPtrResponse, error) {
	payload := AddressPtrRequest{}

	if fqdn != nil && strings.TrimSpace(*fqdn) != "" {
		normalized := strings.TrimSpace(strings.TrimSuffix(*fqdn, "."))
		payload.RDNS = &normalized
		payload.Description = normalized
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		fmt.Sprintf("%s/backend/com.newvm.network/v1/address/%s/dns/ptr", c.HostURL, url.PathEscape(ipAddress)),
		bytes.NewReader(raw),
	)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	var response AddressPtrResponse
	if len(body) == 0 {
		response.Success = true
		return &response, nil
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	if !response.Success {
		return nil, fmt.Errorf("API returned success=false while updating PTR for %q", ipAddress)
	}

	return &response, nil
}
