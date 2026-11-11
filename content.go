package freeastro

import (
	"encoding/json"
	"mime"
	"net/http"
	"strings"
)

// Content is a response body returned as sent, for operations that answer with images
// (SVG, PNG), documents (PDF, Markdown) or several content types.
type Content struct {
	// ContentType is the response's Content-Type header, e.g. "image/svg+xml".
	ContentType string
	// Data is the response body.
	Data []byte
}

// MediaType returns the content type without parameters, e.g. "image/svg+xml".
func (c *Content) MediaType() string {
	mediaType, _, err := mime.ParseMediaType(c.ContentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.SplitN(c.ContentType, ";", 2)[0]))
	}
	return mediaType
}

// IsJSON reports whether the body is JSON.
func (c *Content) IsJSON() bool {
	mt := c.MediaType()
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

// DecodeJSON decodes a JSON body into v.
func (c *Content) DecodeJSON(v any) error {
	if err := json.Unmarshal(c.Data, v); err != nil {
		return &DecodeError{StatusCode: http.StatusOK, ContentType: c.ContentType, Body: c.Data, Err: err}
	}
	return nil
}

// String returns the body as text (useful for SVG and Markdown).
func (c *Content) String() string { return string(c.Data) }

func newContent(resp *http.Response, body []byte) *Content {
	c := &Content{Data: body}
	if resp != nil {
		c.ContentType = resp.Header.Get("Content-Type")
	}
	return c
}

// decoded returns the value the low-level client decoded, or decodes the body itself when
// the server sent JSON under another content type.
func decoded[T any](v *T, resp *http.Response, body []byte) (*T, error) {
	if v != nil {
		return v, nil
	}
	out := new(T)
	if err := json.Unmarshal(body, out); err != nil {
		return nil, &DecodeError{StatusCode: statusOf(resp), ContentType: contentTypeOf(resp), Body: body, Err: err}
	}
	return out, nil
}

// rawJSON returns the body of an operation whose response schema is not documented.
func rawJSON(resp *http.Response, body []byte) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	if !json.Valid(body) {
		return nil, &DecodeError{StatusCode: statusOf(resp), ContentType: contentTypeOf(resp), Body: body, Err: errInvalidJSON}
	}
	return json.RawMessage(body), nil
}

type jsonError string

func (e jsonError) Error() string { return string(e) }

const errInvalidJSON = jsonError("response body is not valid JSON")

func statusOf(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func contentTypeOf(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return resp.Header.Get("Content-Type")
}
