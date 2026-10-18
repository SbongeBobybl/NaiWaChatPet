// Package aiorganization defines the bounded, metadata-only DeskBox protocol.
package aiorganization

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const Version = 1
const MaxMessage = 64 << 10
const MaxBatch = 40

var ErrInvalid = errors.New("invalid organization message")
var idPattern = regexp.MustCompile("^[a-zA-Z0-9_-]{1,64}$")

// Extension is model metadata only, never a path. Folders often carry a
// version-like suffix ("…-v2.0.1-InternalTest-x64") and files may use suffixes
// longer than 16 characters, so only separators, control characters and absurd
// lengths are rejected.
func safeExtension(value string) bool {
	if len([]rune(value)) > 32 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '/' || r == '\\' || r == ':' {
			return false
		}
	}
	return true
}

type Item struct {
	ItemID      string `json:"itemId"`
	DisplayName string `json:"displayName"`
	Extension   string `json:"extension"`
	Kind        string `json:"kind"`
	IsProtected bool   `json:"isProtected"`
}
type Category struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}
type Request struct {
	ProposeCategories bool       `json:"proposeCategories,omitempty"`
	SchemaVersion     int        `json:"schemaVersion"`
	RequestID         string     `json:"requestId"`
	Session           string     `json:"session"`
	Token             string     `json:"token"`
	Operation         string     `json:"operation"`
	Consent           bool       `json:"consent"`
	Items             []Item     `json:"items,omitempty"`
	Categories        []Category `json:"categories,omitempty"`
	Instruction       string     `json:"instruction,omitempty"`
	CancelRequestID   string     `json:"cancelRequestId,omitempty"`
	Carry             *CarryMove `json:"carry,omitempty"`
	CarryRequestID    string     `json:"carryRequestId,omitempty"`
	Succeeded         bool       `json:"succeeded,omitempty"`
}

type CarryMove struct {
	SourcePaths     []string `json:"sourcePaths,omitempty"`
	SourcePath      string   `json:"sourcePath"`
	DestinationX    float64  `json:"destinationX"`
	DestinationY    float64  `json:"destinationY"`
	DestinationName string   `json:"destinationName"`
}
type CarryHandler interface {
	Carry(context.Context, Request) error
	CompleteCarry(context.Context, Request) error
}
type Suggestion struct {
	ItemID     string  `json:"itemId"`
	CategoryID string  `json:"categoryId"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}
type Intent struct {
	CategoryID string `json:"categoryId"`
	Position   string `json:"position"`
	Collapsed  bool   `json:"collapsed"`
	Locked     bool   `json:"locked"`
}
type Result struct {
	Categories []Category   `json:"categories,omitempty"`
	Items      []Suggestion `json:"items"`
	Intents    []Intent     `json:"intents"`
}
type Response struct {
	SchemaVersion int     `json:"schemaVersion"`
	RequestID     string  `json:"requestId"`
	Session       string  `json:"session"`
	Status        string  `json:"status"`
	Error         string  `json:"error,omitempty"`
	Configured    bool    `json:"configured"`
	Result        *Result `json:"result,omitempty"`
}
type Handler interface {
	Configured() bool
	Classify(context.Context, Request) (Result, error)
	ParseIntent(context.Context, Request) (Result, error)
}

func SafeText(s string, max int) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || len([]rune(s)) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune("/\\:", r) {
			return false
		}
	}
	return true
}

// safeReason validates display-only model prose. It is rendered as text and is
// never used as a path, so punctuation such as "/" or ":" is allowed.
func safeReason(s string) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || len([]rune(s)) > 160 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// NormalizeCategoryNames makes model-proposed labels safe as future folder
// names while keeping them readable, and removes duplicates the client would
// otherwise reject.
func NormalizeCategoryNames(categories []Category) []Category {
	out := make([]Category, len(categories))
	used := map[string]int{}
	for index, category := range categories {
		name := sanitizeCategoryName(category.DisplayName)
		if seen, ok := used[name]; ok {
			used[name] = seen + 1
			name = fmt.Sprintf("%s-%d", name, seen+1)
		} else {
			used[name] = 1
		}
		category.DisplayName = name
		out[index] = category
	}
	return out
}

func sanitizeCategoryName(name string) string {
	var builder strings.Builder
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			builder.WriteRune('-')
		} else {
			builder.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(builder.String())
	cleaned = strings.TrimRight(cleaned, " .")
	if cleaned == "" {
		cleaned = "未命名"
	}
	if runes := []rune(cleaned); len(runes) > 40 {
		cleaned = string(runes[:40])
	}
	return cleaned
}
func (r Request) Validate() error {
	if r.SchemaVersion != Version || !idPattern.MatchString(r.RequestID) {
		return ErrInvalid
	}
	switch r.Operation {
	case "health":
	case "carry":
		if r.Carry == nil || len(r.Carry.SourcePath) == 0 || len(r.Carry.SourcePath) > 1024 || strings.ContainsRune(r.Carry.SourcePath, 0) ||
			!SafeText(r.Carry.DestinationName, 80) || math.IsNaN(r.Carry.DestinationX) || math.IsNaN(r.Carry.DestinationY) ||
			math.Abs(r.Carry.DestinationX) > 100000 || math.Abs(r.Carry.DestinationY) > 100000 {
			return ErrInvalid
		}
		if len(r.Carry.SourcePaths) > 40 {
			return ErrInvalid
		}
		seenPaths := map[string]bool{}
		for _, path := range r.Carry.SourcePaths {
			key := strings.ToLower(path)
			if len(path) == 0 || len(path) > 1024 || strings.ContainsRune(path, 0) || seenPaths[key] {
				return ErrInvalid
			}
			seenPaths[key] = true
		}
	case "complete_carry":
		if !idPattern.MatchString(r.CarryRequestID) {
			return ErrInvalid
		}
	case "cancel":
		if !idPattern.MatchString(r.CancelRequestID) {
			return ErrInvalid
		}
	case "classify", "parse_intent":
		if len(r.Items) > MaxBatch || len(r.Categories) == 0 || len(r.Categories) > 32 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, c := range r.Categories {
			if !idPattern.MatchString(c.ID) || seen[c.ID] || !SafeText(c.DisplayName, 40) {
				return ErrInvalid
			}
			seen[c.ID] = true
		}
		seen = map[string]bool{}
		for _, i := range r.Items {
			if !idPattern.MatchString(i.ItemID) || seen[i.ItemID] || i.IsProtected || !SafeText(i.DisplayName, 255) || !safeExtension(i.Extension) || (i.Kind != "file" && i.Kind != "folder" && i.Kind != "shortcut") {
				return ErrInvalid
			}
			seen[i.ItemID] = true
		}
		if r.Operation == "parse_intent" && (!SafeText(r.Instruction, 500) || len(r.Items) > 0) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func ValidateResult(r Request, result Result) error {
	if r.ProposeCategories {
		if r.Operation != "classify" || len(result.Categories) < 6 || len(result.Categories) > 10 {
			return ErrInvalid
		}
		names := map[string]bool{}
		keys := map[string]bool{}
		for _, c := range result.Categories {
			if !idPattern.MatchString(c.ID) || !SafeText(c.DisplayName, 40) || keys[c.ID] || names[c.DisplayName] {
				return ErrInvalid
			}
			keys[c.ID] = true
			names[c.DisplayName] = true
		}
		r.Categories = result.Categories
	} else if len(result.Categories) > 0 {
		return ErrInvalid
	}

	if len(result.Items) > len(r.Items) || len(result.Intents) > len(r.Categories) {
		return ErrInvalid
	}
	ids := map[string]bool{}
	cats := map[string]bool{}
	seen := map[string]bool{}
	for _, i := range r.Items {
		ids[i.ItemID] = !i.IsProtected
	}
	for _, c := range r.Categories {
		cats[c.ID] = true
	}
	for _, s := range result.Items {
		if !ids[s.ItemID] || seen[s.ItemID] || !cats[s.CategoryID] || math.IsNaN(s.Confidence) || math.IsInf(s.Confidence, 0) || s.Confidence < 0 || s.Confidence > 1 || !safeReason(s.Reason) {
			return ErrInvalid
		}
		seen[s.ItemID] = true
	}
	seen = map[string]bool{}
	for _, i := range result.Intents {
		if !cats[i.CategoryID] || seen[i.CategoryID] {
			return ErrInvalid
		}
		seen[i.CategoryID] = true
		switch i.Position {
		case "keep", "top_left", "top_right", "bottom_left", "bottom_right":
		default:
			return ErrInvalid
		}
	}
	if r.Operation == "classify" && len(result.Intents) > 0 || r.Operation == "parse_intent" && len(result.Items) > 0 {
		return ErrInvalid
	}
	return nil
}
func DecodeStrict(data []byte, dst any) error {
	if len(data) > MaxMessage {
		return ErrInvalid
	}
	guard := json.NewDecoder(bytes.NewReader(data))
	if validateJSONValue(guard, 0) != nil {
		return ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ErrInvalid
	}
	if dec.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}

// Reject duplicate object keys and deep input before the typed decoder runs.
func validateJSONValue(d *json.Decoder, depth int) error {
	if depth > 16 {
		return ErrInvalid
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return ErrInvalid
			}
			keys[name] = true
			if err := validateJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := validateJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	_, err = d.Token()
	return err
}
func ReadFrame(r io.Reader, dst any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > MaxMessage {
		return ErrInvalid
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return DecodeStrict(body, dst)
}
func WriteFrame(w io.Writer, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body) > MaxMessage {
		return ErrInvalid
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(body)))
	if _, err = w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}
