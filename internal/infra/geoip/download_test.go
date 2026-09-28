package geoip

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/oschwald/geoip2-golang"
)

func TestDownloadGeoIPDatabaseInstallsVerifiedDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoLite2-City.mmdb")
	server := serveBytes(t, buildTestMMDB(GeoIPDBType, map[string]any{"country": map[string]any{"iso_code": "AU"}}))

	if err := DownloadGeoIPDatabase(server.URL, path, GeoIPDBType); err != nil {
		t.Fatalf("DownloadGeoIPDatabase: %v", err)
	}
	db, err := geoip2.Open(path)
	if err != nil {
		t.Fatalf("installed database does not open: %v", err)
	}
	defer func() { _ = db.Close() }()
	record, err := db.City(net.IPv4(1, 1, 1, 1))
	if err != nil || record.Country.IsoCode != "AU" {
		t.Fatalf("lookup = %+v (err %v), want the served record", record, err)
	}
	assertOnlyFile(t, dir, "GeoLite2-City.mmdb")
}

// A download that is not a complete database of the expected type must
// never replace the active database, and must not leave its temp file.
func TestDownloadGeoIPDatabaseRejectsUnverifiedDownloads(t *testing.T) {
	city := buildTestMMDB(GeoIPDBType, map[string]any{})
	corruptTree := append([]byte(nil), city...)
	// Both records point far past the end of the file.
	copy(corruptTree, []byte{0, 0x03, 0xF9, 0, 0x03, 0xF9})
	for name, body := range map[string][]byte{
		"error page":         []byte("<html>rate limited</html>"),
		"empty":              {},
		"truncated":          city[:len(city)/2],
		"last byte missing":  city[:len(city)-1],
		"wrong database":     buildTestMMDB(GeoIPASNDBType, map[string]any{}),
		"unknown database":   buildTestMMDB("Example-Other", map[string]any{}),
		"corrupt tree":       corruptTree,
		"country for a city": buildTestMMDB("GeoLite2-Country", map[string]any{}),
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "GeoLite2-City.mmdb")
			active := []byte("previous database")
			if err := os.WriteFile(path, active, 0o644); err != nil {
				t.Fatal(err)
			}

			err := DownloadGeoIPDatabase(serveBytes(t, body).URL, path, GeoIPDBType)

			if err == nil {
				t.Fatal("unverified download was accepted")
			}
			t.Logf("rejected: %v", err)
			if current, _ := os.ReadFile(path); !bytes.Equal(current, active) {
				t.Fatal("unverified download replaced the active database")
			}
			assertOnlyFile(t, dir, "GeoLite2-City.mmdb")
		})
	}
}

func TestDownloadGeoIPDatabaseVerifiesASNEdition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoLite2-ASN.mmdb")
	asn := buildTestMMDB(GeoIPASNDBType, map[string]any{"autonomous_system_number": uint32(13335)})

	if err := DownloadGeoIPDatabase(serveBytes(t, asn).URL, path, GeoIPASNDBType); err != nil {
		t.Fatalf("ASN download: %v", err)
	}
	if err := DownloadGeoIPDatabase(serveBytes(t, buildTestMMDB(GeoIPDBType, map[string]any{})).URL, path, GeoIPASNDBType); err == nil {
		t.Fatal("a City database was accepted as the ASN database")
	}
}

func serveBytes(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func assertOnlyFile(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("directory holds %v, want only %s", names, name)
	}
}

// buildTestMMDB encodes a minimal IPv4 MaxMind DB: one search-tree node
// whose both records point at the single data record.
func buildTestMMDB(databaseType string, record map[string]any) []byte {
	var data, metadata mmdbEncoder
	data.value(record)
	metadata.value(map[string]any{
		"binary_format_major_version": uint16(2),
		"binary_format_minor_version": uint16(0),
		"build_epoch":                 uint64(1700000000),
		"database_type":               databaseType,
		"description":                 map[string]any{"en": "test database"},
		"ip_version":                  uint16(4),
		"languages":                   []string{"en"},
		"node_count":                  uint32(1),
		"record_size":                 uint16(24),
	})
	// A record value above node_count points into the data section at
	// offset value - node_count - 16 (the separator).
	const firstRecord = 1 + 16
	var db bytes.Buffer
	db.Write([]byte{0, 0, firstRecord, 0, 0, firstRecord})
	db.Write(make([]byte, 16))
	db.Write(data.Bytes())
	db.WriteString("\xAB\xCD\xEFMaxMind.com")
	db.Write(metadata.Bytes())
	return db.Bytes()
}

// mmdbEncoder writes the MaxMind DB data format for the handful of types
// the test databases use; every size stays below 29.
type mmdbEncoder struct {
	bytes.Buffer
}

func (e *mmdbEncoder) control(typ byte, size int) {
	if size >= 29 {
		panic("test encoder supports sizes below 29 only")
	}
	if typ <= 7 {
		e.WriteByte(typ<<5 | byte(size))
		return
	}
	// Extended type: zero type bits, then the type number minus 7.
	e.WriteByte(byte(size))
	e.WriteByte(typ - 7)
}

func (e *mmdbEncoder) unsigned(typ byte, v uint64) {
	var raw []byte
	for ; v > 0; v >>= 8 {
		raw = append([]byte{byte(v)}, raw...)
	}
	e.control(typ, len(raw))
	e.Write(raw)
}

func (e *mmdbEncoder) value(v any) {
	switch v := v.(type) {
	case string:
		e.control(2, len(v))
		e.WriteString(v)
	case uint16:
		e.unsigned(5, uint64(v))
	case uint32:
		e.unsigned(6, uint64(v))
	case uint64:
		e.unsigned(9, v)
	case []string:
		e.control(11, len(v))
		for _, item := range v {
			e.value(item)
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		e.control(7, len(v))
		for _, key := range keys {
			e.value(key)
			e.value(v[key])
		}
	default:
		panic("unsupported test value")
	}
}
