package opml

import (
	"bytes"
	"encoding/xml"
	"os"
)

// sanitizeOPMLData fixes common XML violations found in real-world OPML exports:
//   - unescaped < and > inside attribute values
//   - bare & not part of a valid entity
//   - C-style \" escape inside double-quoted attribute values
//
// It walks the raw bytes with a simple state machine that activates only
// inside ="..." attribute values to avoid mangling tag syntax.
func sanitizeOPMLData(data []byte) []byte {
	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		// Detect start of a double-quoted attribute value: ="
		if i+1 < len(data) && data[i] == '=' && data[i+1] == '"' {
			out = append(out, '=', '"')
			i += 2
			// Scan until closing (unescaped) "
			for i < len(data) {
				b := data[i]
				switch {
				case b == '\\' && i+1 < len(data) && data[i+1] == '"':
					// C-style \" → proper XML escape
					out = append(out, '&', 'q', 'u', 'o', 't', ';')
					i += 2
				case b == '"':
					// End of attribute value
					out = append(out, '"')
					i++
					goto nextOuter
				case b == '<':
					out = append(out, '&', 'l', 't', ';')
					i++
				case b == '>':
					out = append(out, '&', 'g', 't', ';')
					i++
				case b == '&':
					if isXMLEntity(data, i) {
						out = append(out, b)
						i++
					} else {
						out = append(out, '&', 'a', 'm', 'p', ';')
						i++
					}
				default:
					out = append(out, b)
					i++
				}
			}
		} else {
			out = append(out, data[i])
			i++
		}
	nextOuter:
	}
	return out
}

// isXMLEntity reports whether data[i] begins a valid XML entity reference
// (&amp; &lt; &gt; &quot; &apos; &#N; &#xH;).
func isXMLEntity(data []byte, i int) bool {
	if data[i] != '&' {
		return false
	}
	rest := data[i:]
	semi := bytes.IndexByte(rest, ';')
	if semi < 1 || semi > 12 {
		return false
	}
	entity := rest[:semi+1]
	for _, valid := range [][]byte{
		[]byte("&amp;"), []byte("&lt;"), []byte("&gt;"),
		[]byte("&quot;"), []byte("&apos;"),
	} {
		if bytes.Equal(entity, valid) {
			return true
		}
	}
	// Numeric: &#NNN; or &#xHHH;
	if len(entity) > 3 && entity[1] == '#' {
		body := entity[2 : len(entity)-1]
		if len(body) == 0 {
			return false
		}
		if body[0] == 'x' || body[0] == 'X' {
			body = body[1:]
			for _, c := range body {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
					return false
				}
			}
			return len(body) > 0
		}
		for _, c := range body {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	return false
}

// OPML represents the root OPML document.
type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Body    Body     `xml:"body"`
}

type Body struct {
	Outlines []Outline `xml:"outline"`
}

// Outline represents a single feed or folder in the OPML structure.
type Outline struct {
	Text     string    `xml:"text,attr"`
	Title    string    `xml:"title,attr"`
	Type     string    `xml:"type,attr"`
	XMLURL   string    `xml:"xmlUrl,attr"`
	HTMLURL  string    `xml:"htmlUrl,attr"`
	Category string    `xml:"category,attr"`
	Outlines []Outline `xml:"outline"`
}

// FeedEntry is a flattened feed parsed from OPML.
type FeedEntry struct {
	Title    string
	URL      string // xmlUrl
	SiteURL  string // htmlUrl
	Category string
}

// Parse reads an OPML file and returns a flat list of feed entries.
func Parse(filePath string) ([]FeedEntry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	data = sanitizeOPMLData(data)

	var opml OPML
	if err := xml.Unmarshal(data, &opml); err != nil {
		return nil, err
	}

	var entries []FeedEntry
	flatten(opml.Body.Outlines, "", &entries)
	return entries, nil
}

func flatten(outlines []Outline, parentCategory string, entries *[]FeedEntry) {
	for _, o := range outlines {
		category := parentCategory
		if o.Category != "" {
			category = o.Category
		} else if o.XMLURL == "" && o.Text != "" {
			// This is a folder node, use its text as category
			category = o.Text
		}

		if o.XMLURL != "" {
			title := o.Title
			if title == "" {
				title = o.Text
			}
			*entries = append(*entries, FeedEntry{
				Title:    title,
				URL:      o.XMLURL,
				SiteURL:  o.HTMLURL,
				Category: category,
			})
		}

		if len(o.Outlines) > 0 {
			flatten(o.Outlines, category, entries)
		}
	}
}
