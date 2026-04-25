// Package output renders structured values to an io.Writer in one of three
// formats: table, json, or yaml.
//
// The Format constants (Table, JSON, YAML) correspond to the values accepted
// by the --format flag defined in the cli package. Render should be called
// with the format value obtained from viper ("format" key) after the root
// command is constructed.
//
// Table rendering is driven by the `table` struct tag. Only fields with a
// non-empty, non-"-" table tag are included. The tag value becomes the column
// header. Fields without a table tag are silently omitted.
//
//	type Item struct {
//	    ID   string `table:"ID"   json:"id"`
//	    Name string `table:"Name" json:"name"`
//	    internal string  // no tag — not rendered
//	}
//
// Render accepts both a single struct and a slice of structs for table mode.
// For JSON and YAML, v is passed directly to the respective encoder.
//
// Note: an empty slice produces no output at all — not even a header row.
// If callers need to show "no results" messaging, check len before calling Render.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Format is the output format specifier. Use the package-level constants
// Table, JSON, and YAML; do not construct arbitrary string values.
type Format = string

const (
	JSON  Format = "json"  // JSON renders v as indented JSON.
	YAML  Format = "yaml"  // YAML renders v as YAML.
	Table Format = "table" // Table renders v using struct `table:""` tags.
)

// RegisterFlags adds the --format persistent flag to cmd and binds it to the
// "format" key in v. The default value is "table".
func RegisterFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.PersistentFlags().String("format", "table", "Output format (table, json, yaml)")
	_ = v.BindPFlag("format", cmd.PersistentFlags().Lookup("format"))
}

// Render writes v to w in the requested format.
//
// For Table format, v may be a struct or a slice of structs; only fields
// tagged with `table:"<header>"` are included. A slice with zero elements
// produces no output. Pointer-to-struct elements in a slice are dereferenced
// automatically.
//
// For JSON and YAML formats, v is passed directly to the respective encoder;
// any JSON/YAML-serialisable value is accepted.
//
// Returns an error if format is not one of the three recognized constants, or
// if encoding fails.
func Render(w io.Writer, format Format, v any) error {
	switch format {
	case JSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	case YAML:
		return yaml.NewEncoder(w).Encode(v)
	case Table:
		return renderTable(w, v)
	default:
		return fmt.Errorf("unknown output format %q (valid: json, yaml, table)", format)
	}
}

func renderTable(w io.Writer, v any) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		if rv.Len() == 0 {
			return nil
		}
		headers, _ := tableFields(rv.Index(0).Type())
		fmt.Fprintln(tw, strings.Join(headers, "\t"))
		for i := range rv.Len() {
			_, idxs := tableFields(rv.Index(i).Type())
			row := make([]string, len(idxs))
			for j, idx := range idxs {
				row[j] = fmt.Sprintf("%v", rv.Index(i).Field(idx))
			}
			fmt.Fprintln(tw, strings.Join(row, "\t"))
		}
		return nil
	}
	headers, idxs := tableFields(rv.Type())
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	row := make([]string, len(idxs))
	for j, idx := range idxs {
		row[j] = fmt.Sprintf("%v", rv.Field(idx))
	}
	fmt.Fprintln(tw, strings.Join(row, "\t"))
	return nil
}

// tableFields returns the `table` tag values (as column headers) and the
// corresponding field indices for the given struct type. Fields without a
// table tag, or with tag value "-", are excluded.
func tableFields(t reflect.Type) (headers []string, indices []int) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for i := range t.NumField() {
		if tag := t.Field(i).Tag.Get("table"); tag != "" && tag != "-" {
			headers = append(headers, tag)
			indices = append(indices, i)
		}
	}
	return
}
