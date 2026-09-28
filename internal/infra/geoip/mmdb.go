// Package geoip reads the local MaxMind GeoLite2 databases: it downloads
// them on first start, verifying a download before it replaces the active
// file, and derives the country, region, city and network of a client IP
// for the request metadata the logs and audit records carry. The lookups
// are local, so no client address leaves the server.
package geoip

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
)

const (
	// GeoIPDBURL and GeoIPASNDBURL are the mirror the City and ASN
	// databases are downloaded from when they are missing.
	GeoIPDBURL    = "https://raw.githubusercontent.com/adysec/IP_database/main/geolite/GeoLite2-City.mmdb"
	GeoIPASNDBURL = "https://raw.githubusercontent.com/adysec/IP_database/main/geolite/GeoLite2-ASN.mmdb"

	// Database types the downloads must declare in their metadata. The
	// mirror publishes no checksums, so the file itself is what gets
	// verified before it may replace the active database.
	GeoIPDBType    = "GeoLite2-City"
	GeoIPASNDBType = "GeoLite2-ASN"
)

// IPLocation holds the open City database and, when it could be opened,
// the ASN database. The readers are shared by every request and stay open
// for the life of the process.
type IPLocation struct {
	Path    string
	DB      *geoip2.Reader
	ASNPath string
	ASNDB   *geoip2.Reader
}

// NewIPLocation opens the City database at path and the ASN database next
// to it, downloading either when it is missing. The City database is
// required; without the ASN database the network organization is omitted.
func NewIPLocation(path string) (*IPLocation, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Infof("[GeoIP] Database not found, downloading from %s", GeoIPDBURL)
		err := DownloadGeoIPDatabase(GeoIPDBURL, path, GeoIPDBType)
		if err != nil {
			logger.Errorf("[GeoIP] Failed to download database: %v", err.Error())
			return nil, err
		}
		logger.Infof("[GeoIP] Database downloaded successfully")
	}

	db, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}

	ipLoc := &IPLocation{Path: path, DB: db}
	asnPath := filepath.Join(filepath.Dir(path), "GeoLite2-ASN.mmdb")
	ipLoc.ASNPath = asnPath
	if _, err := os.Stat(asnPath); os.IsNotExist(err) {
		logger.Infof("[GeoIP] ASN database not found, downloading from %s", GeoIPASNDBURL)
		if err := DownloadGeoIPDatabase(GeoIPASNDBURL, asnPath, GeoIPASNDBType); err != nil {
			// ASN enrichment is optional. A transient download problem must not
			// turn logging metadata into an application startup dependency.
			logger.Errorf("[GeoIP] Failed to download ASN database; network organization will be omitted: %v", err)
			return ipLoc, nil
		}
		logger.Infof("[GeoIP] ASN database downloaded successfully")
	}
	if asnDB, err := geoip2.Open(asnPath); err != nil {
		logger.Errorf("[GeoIP] Failed to open ASN database; network organization will be omitted: %v", err)
	} else {
		ipLoc.ASNDB = asnDB
	}
	return ipLoc, nil
}

// Enrich performs at most one City and one ASN lookup for a public client IP.
// MMDB lookup errors are treated as misses so request handling remains
// independent from optional risk metadata.
func (ipLoc *IPLocation) Enrich(metadata requestmeta.Metadata) requestmeta.Metadata {
	ip := net.ParseIP(metadata.ClientIP)
	if ipLoc == nil || ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return requestmeta.Normalize(metadata)
	}
	if ipLoc.DB != nil {
		if record, err := ipLoc.DB.City(ip); err == nil && record != nil {
			metadata.IPCountryCode = record.Country.IsoCode
			metadata.IPCountry = preferredGeoName(record.Country.Names)
			if len(record.Subdivisions) > 0 {
				metadata.IPRegion = preferredGeoName(record.Subdivisions[0].Names)
			}
			metadata.IPCity = preferredGeoName(record.City.Names)
		}
	}
	if ipLoc.ASNDB != nil {
		if record, err := ipLoc.ASNDB.ASN(ip); err == nil && record != nil {
			metadata.IPASN = record.AutonomousSystemNumber
			metadata.IPASOrganization = record.AutonomousSystemOrganization
		}
	}
	return requestmeta.Normalize(metadata)
}

func preferredGeoName(names map[string]string) string {
	for _, language := range []string{"en", "zh-CN", "zh"} {
		if name := names[language]; name != "" {
			return name
		}
	}
	return ""
}

// DownloadGeoIPDatabase fetches a database to path. The file only replaces
// path once it has been verified as a complete database of databaseType.
func DownloadGeoIPDatabase(url, path, databaseType string) error {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		logger.Errorf("[GeoIP] Failed to create directory: %v", err.Error())
		return err
	}

	// Write into a sibling temporary file so a timeout or truncated response
	// never leaves a corrupt database that blocks the next startup retry.
	out, err := os.CreateTemp(filepath.Dir(path), ".geoip-*.tmp")
	if err != nil {
		return err
	}
	tempPath := out.Name()
	committed := false
	defer func() {
		_ = out.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	// The body is only read; closing it cannot lose data.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download GeoIP database: HTTP %d", resp.StatusCode)
	}

	const maxGeoIPDatabaseSize = int64(256 << 20)
	written, err := io.Copy(out, io.LimitReader(resp.Body, maxGeoIPDatabaseSize+1))
	if err != nil {
		return err
	}
	if written > maxGeoIPDatabaseSize {
		return fmt.Errorf("download GeoIP database: response exceeds %d bytes", maxGeoIPDatabaseSize)
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := verifyGeoIPDatabase(tempPath, databaseType); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

// verifyGeoIPDatabase checks a downloaded file before it may replace the
// active database. It must open as a MaxMind DB — the metadata section sits
// at the very end of the file, so a truncated transfer fails here — declare
// the expected database type, and answer a lookup through its search tree
// and data section.
func verifyGeoIPDatabase(path, databaseType string) error {
	db, err := geoip2.Open(path)
	if db != nil {
		// An unknown database type still returns an open reader.
		defer func() { _ = db.Close() }()
	}
	if err != nil {
		return fmt.Errorf("downloaded GeoIP database is invalid: %w", err)
	}
	if got := db.Metadata().DatabaseType; got != databaseType {
		return fmt.Errorf("downloaded GeoIP database type is %q, want %q", got, databaseType)
	}
	probe := net.IPv4(1, 1, 1, 1)
	if databaseType == GeoIPASNDBType {
		_, err = db.ASN(probe)
	} else {
		_, err = db.City(probe)
	}
	if err != nil {
		return fmt.Errorf("downloaded GeoIP database failed a lookup: %w", err)
	}
	return nil
}
