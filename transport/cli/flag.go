package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// EnumMapping maps a CLI-facing string value to a protobuf enum number.
type EnumMapping struct {
	CLIValue string
	Number   int32
}

// Int32Value implements [flag.Value] for int32 fields.
type Int32Value struct {
	ptr *int32
}

// NewInt32Value returns a new [Int32Value] bound to the given pointer.
func NewInt32Value(ptr *int32) *Int32Value {
	return &Int32Value{ptr: ptr}
}

func (v *Int32Value) Set(s string) error {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return err
	}
	*v.ptr = int32(n)
	return nil
}

func (v *Int32Value) String() string {
	if v == nil || v.ptr == nil {
		return "0"
	}
	return strconv.FormatInt(int64(*v.ptr), 10)
}

// Int64Value implements [flag.Value] for int64 fields.
type Int64Value struct {
	ptr *int64
}

// NewInt64Value returns a new [Int64Value] bound to the given pointer.
func NewInt64Value(ptr *int64) *Int64Value {
	return &Int64Value{ptr: ptr}
}

func (v *Int64Value) Set(s string) error {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*v.ptr = n
	return nil
}

func (v *Int64Value) String() string {
	if v == nil || v.ptr == nil {
		return "0"
	}
	return strconv.FormatInt(*v.ptr, 10)
}

// Uint32Value implements [flag.Value] for uint32 fields.
type Uint32Value struct {
	ptr *uint32
}

// NewUint32Value returns a new [Uint32Value] bound to the given pointer.
func NewUint32Value(ptr *uint32) *Uint32Value {
	return &Uint32Value{ptr: ptr}
}

func (v *Uint32Value) Set(s string) error {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return err
	}
	*v.ptr = uint32(n)
	return nil
}

func (v *Uint32Value) String() string {
	if v == nil || v.ptr == nil {
		return "0"
	}
	return strconv.FormatUint(uint64(*v.ptr), 10)
}

// Uint64Value implements [flag.Value] for uint64 fields.
type Uint64Value struct {
	ptr *uint64
}

// NewUint64Value returns a new [Uint64Value] bound to the given pointer.
func NewUint64Value(ptr *uint64) *Uint64Value {
	return &Uint64Value{ptr: ptr}
}

func (v *Uint64Value) Set(s string) error {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*v.ptr = n
	return nil
}

func (v *Uint64Value) String() string {
	if v == nil || v.ptr == nil {
		return "0"
	}
	return strconv.FormatUint(*v.ptr, 10)
}

// Float32Value implements [flag.Value] for float fields.
type Float32Value struct {
	ptr *float32
}

// NewFloat32Value returns a new [Float32Value] bound to the given pointer.
func NewFloat32Value(ptr *float32) *Float32Value {
	return &Float32Value{ptr: ptr}
}

func (v *Float32Value) Set(s string) error {
	n, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return err
	}
	if n > math.MaxFloat32 || n < -math.MaxFloat32 {
		return fmt.Errorf("value %s overflows float32", s)
	}
	*v.ptr = float32(n)
	return nil
}

func (v *Float32Value) String() string {
	if v == nil || v.ptr == nil {
		return "0"
	}
	return strconv.FormatFloat(float64(*v.ptr), 'g', -1, 32)
}

// EnumValue implements [flag.Value] for protobuf enum fields.
// It accepts case-insensitive stripped enum value names.
type EnumValue struct {
	ptr      *int32
	mappings []EnumMapping
	typeName string
}

// NewEnumValue returns a new [EnumValue] bound to the given pointer.
func NewEnumValue(ptr *int32, mappings []EnumMapping, typeName string) *EnumValue {
	return &EnumValue{ptr: ptr, mappings: mappings, typeName: typeName}
}

func (v *EnumValue) Set(s string) error {
	lower := strings.ToLower(s)
	for _, m := range v.mappings {
		if strings.ToLower(m.CLIValue) == lower {
			*v.ptr = m.Number
			return nil
		}
	}
	valid := make([]string, 0, len(v.mappings))
	for _, m := range v.mappings {
		valid = append(valid, m.CLIValue)
	}
	return fmt.Errorf("invalid %s value %q (valid: %s)", v.typeName, s, strings.Join(valid, ", "))
}

func (v *EnumValue) String() string {
	if v == nil || v.ptr == nil || *v.ptr == 0 {
		return ""
	}
	for _, m := range v.mappings {
		if m.Number == *v.ptr {
			return m.CLIValue
		}
	}
	return ""
}

// StringSliceValue implements [flag.Value] for repeated string fields.
// Each invocation of Set appends to the slice.
type StringSliceValue struct {
	ptr *[]string
}

// NewStringSliceValue returns a new [StringSliceValue] bound to the given pointer.
func NewStringSliceValue(ptr *[]string) *StringSliceValue {
	return &StringSliceValue{ptr: ptr}
}

func (v *StringSliceValue) Set(s string) error {
	*v.ptr = append(*v.ptr, s)
	return nil
}

func (v *StringSliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	return "[" + strings.Join(*v.ptr, ",") + "]"
}

// BoolSliceValue implements [flag.Value] for repeated bool fields.
// Each invocation of Set appends to the slice.
type BoolSliceValue struct {
	ptr *[]bool
}

// NewBoolSliceValue returns a new [BoolSliceValue] bound to the given pointer.
func NewBoolSliceValue(ptr *[]bool) *BoolSliceValue {
	return &BoolSliceValue{ptr: ptr}
}

func (v *BoolSliceValue) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*v.ptr = append(*v.ptr, b)
	return nil
}

func (v *BoolSliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, b := range *v.ptr {
		parts[i] = strconv.FormatBool(b)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Int32SliceValue implements [flag.Value] for repeated int32 fields.
// Each invocation of Set appends to the slice.
type Int32SliceValue struct {
	ptr *[]int32
}

// NewInt32SliceValue returns a new [Int32SliceValue] bound to the given pointer.
func NewInt32SliceValue(ptr *[]int32) *Int32SliceValue {
	return &Int32SliceValue{ptr: ptr}
}

func (v *Int32SliceValue) Set(s string) error {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return err
	}
	*v.ptr = append(*v.ptr, int32(n))
	return nil
}

func (v *Int32SliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, n := range *v.ptr {
		parts[i] = strconv.FormatInt(int64(n), 10)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Int64SliceValue implements [flag.Value] for repeated int64 fields.
// Each invocation of Set appends to the slice.
type Int64SliceValue struct {
	ptr *[]int64
}

// NewInt64SliceValue returns a new [Int64SliceValue] bound to the given pointer.
func NewInt64SliceValue(ptr *[]int64) *Int64SliceValue {
	return &Int64SliceValue{ptr: ptr}
}

func (v *Int64SliceValue) Set(s string) error {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*v.ptr = append(*v.ptr, n)
	return nil
}

func (v *Int64SliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, n := range *v.ptr {
		parts[i] = strconv.FormatInt(n, 10)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Uint32SliceValue implements [flag.Value] for repeated uint32 fields.
// Each invocation of Set appends to the slice.
type Uint32SliceValue struct {
	ptr *[]uint32
}

// NewUint32SliceValue returns a new [Uint32SliceValue] bound to the given pointer.
func NewUint32SliceValue(ptr *[]uint32) *Uint32SliceValue {
	return &Uint32SliceValue{ptr: ptr}
}

func (v *Uint32SliceValue) Set(s string) error {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return err
	}
	*v.ptr = append(*v.ptr, uint32(n))
	return nil
}

func (v *Uint32SliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, n := range *v.ptr {
		parts[i] = strconv.FormatUint(uint64(n), 10)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Uint64SliceValue implements [flag.Value] for repeated uint64 fields.
// Each invocation of Set appends to the slice.
type Uint64SliceValue struct {
	ptr *[]uint64
}

// NewUint64SliceValue returns a new [Uint64SliceValue] bound to the given pointer.
func NewUint64SliceValue(ptr *[]uint64) *Uint64SliceValue {
	return &Uint64SliceValue{ptr: ptr}
}

func (v *Uint64SliceValue) Set(s string) error {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*v.ptr = append(*v.ptr, n)
	return nil
}

func (v *Uint64SliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, n := range *v.ptr {
		parts[i] = strconv.FormatUint(n, 10)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Float32SliceValue implements [flag.Value] for repeated float fields.
// Each invocation of Set appends to the slice.
type Float32SliceValue struct {
	ptr *[]float32
}

// NewFloat32SliceValue returns a new [Float32SliceValue] bound to the given pointer.
func NewFloat32SliceValue(ptr *[]float32) *Float32SliceValue {
	return &Float32SliceValue{ptr: ptr}
}

func (v *Float32SliceValue) Set(s string) error {
	n, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return err
	}
	if n > math.MaxFloat32 || n < -math.MaxFloat32 {
		return fmt.Errorf("value %s overflows float32", s)
	}
	*v.ptr = append(*v.ptr, float32(n))
	return nil
}

func (v *Float32SliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, n := range *v.ptr {
		parts[i] = strconv.FormatFloat(float64(n), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Float64SliceValue implements [flag.Value] for repeated float64 fields.
// Each invocation of Set appends to the slice.
type Float64SliceValue struct {
	ptr *[]float64
}

// NewFloat64SliceValue returns a new [Float64SliceValue] bound to the given pointer.
func NewFloat64SliceValue(ptr *[]float64) *Float64SliceValue {
	return &Float64SliceValue{ptr: ptr}
}

func (v *Float64SliceValue) Set(s string) error {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*v.ptr = append(*v.ptr, n)
	return nil
}

func (v *Float64SliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, n := range *v.ptr {
		parts[i] = strconv.FormatFloat(n, 'g', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// EnumSliceValue implements [flag.Value] for repeated enum fields.
// Each invocation of Set appends to the slice.
type EnumSliceValue struct {
	ptr      *[]int32
	mappings []EnumMapping
	typeName string
}

// NewEnumSliceValue returns a new [EnumSliceValue] bound to the given pointer.
func NewEnumSliceValue(ptr *[]int32, mappings []EnumMapping, typeName string) *EnumSliceValue {
	return &EnumSliceValue{ptr: ptr, mappings: mappings, typeName: typeName}
}

func (v *EnumSliceValue) Set(s string) error {
	lower := strings.ToLower(s)
	for _, m := range v.mappings {
		if strings.ToLower(m.CLIValue) == lower {
			*v.ptr = append(*v.ptr, m.Number)
			return nil
		}
	}
	valid := make([]string, 0, len(v.mappings))
	for _, m := range v.mappings {
		valid = append(valid, m.CLIValue)
	}
	return fmt.Errorf("invalid %s value %q (valid: %s)", v.typeName, s, strings.Join(valid, ", "))
}

func (v *EnumSliceValue) String() string {
	if v == nil || v.ptr == nil || len(*v.ptr) == 0 {
		return "[]"
	}
	parts := make([]string, len(*v.ptr))
	for i, n := range *v.ptr {
		found := false
		for _, m := range v.mappings {
			if m.Number == n {
				parts[i] = m.CLIValue
				found = true
				break
			}
		}
		if !found {
			parts[i] = strconv.FormatInt(int64(n), 10)
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}
