package protobuf

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerate(t *testing.T) {
	w := &bytes.Buffer{}
	err := GenerateProtobufDefinition(w, []interface{}{test{}}, nil, nil)
	assert.NoError(t, err)
	expected := `
message test {
  optional bool boolean = 1;
  required sint64 i = 2;
  required sint32 i32 = 3;
  required sint64 i64 = 4;
  required uint32 u32 = 5;
  required uint64 u64 = 6;
  required sfixed32 sx32 = 7;
  required sfixed64 sx64 = 8;
  required fixed32 ux32 = 9;
  required ufixed64 ux64 = 10;
  required float f32 = 11;
  required double f64 = 12;
  required bytes bytes = 13;
  required bytes array = 14;
  required string string = 15;
  required emb struct = 16;
  optional bool obool = 50;
  optional sint32 oi32 = 51;
  optional sint64 oi64 = 52;
  optional uint32 ou32 = 53;
  optional uint64 ou64 = 54;
  optional float of32 = 55;
  optional double of64 = 56;
  optional bytes obytes = 57;
  optional string ostring = 58;
  optional test ostruct = 59;
  repeated bool sbool = 100 [packed=true];
  repeated sint32 si32 = 101 [packed=true];
  repeated sint64 si64 = 102 [packed=true];
  repeated uint32 su32 = 103 [packed=true];
  repeated uint64 su64 = 104 [packed=true];
  repeated sfixed32 ssx32 = 105 [packed=true];
  repeated sfixed64 ssx64 = 106 [packed=true];
  repeated fixed32 sux32 = 107 [packed=true];
  repeated ufixed64 sux64 = 108 [packed=true];
  repeated float sf32 = 109 [packed=true];
  repeated double sf64 = 110 [packed=true];
  repeated bytes sbytes = 111;
  repeated string sstring = 112;
  repeated emb sstruct = 113;
}

`
	assert.Equal(t, expected, w.String())
}

func TestGeneratePersonExample(t *testing.T) {
	w := &bytes.Buffer{}
	err := GenerateProtobufDefinition(w, []interface{}{Person{}, PhoneNumber{}}, nil, nil)
	assert.NoError(t, err)
	expected := `
message Person {
  required string name = 1;
  required sint32 id = 2;
  optional string email = 3;
  repeated PhoneNumber phone = 4;
}

message PhoneNumber {
  required string number = 1;
  optional uint32 type = 2;
}

`
	assert.Equal(t, expected, w.String())
}

func TestGenerateMapExample(t *testing.T) {
	w := &bytes.Buffer{}
	err := GenerateProtobufDefinition(w, []interface{}{MessageWithMap{}, FloatingPoint{}}, nil, nil)
	assert.NoError(t, err)

	expected := `
message FloatingPoint {
  optional double f = 1;
}

message MessageWithMap {
  required map<uint32, string> name_mapping = 1;
  required map<bool, bytes> byte_mapping = 2;
  required map<sint64, FloatingPoint> msg_mapping = 3;
  required map<string, string> str_to_str = 4;
  required map<string, Inner> struct_mapping = 5;
}

`
	assert.Equal(t, expected, w.String())
}

type TimeStruct struct {
	Created time.Time
	Delay   time.Duration
}

func TestGenerateTimeFields(t *testing.T) {
	w := &bytes.Buffer{}
	err := GenerateProtobufDefinition(w, []interface{}{TimeStruct{}}, nil, nil)
	assert.NoError(t, err)
	expected := `
message TimeStruct {
  required sfixed64 created = 1;
  required sint64 delay = 2;
}

`
	assert.Equal(t, expected, w.String())
}

type EnumType uint32

const (
	EnumValueOne EnumType = iota
	EnumValueTwo
)

type typeWithEnumField struct {
	Value EnumType
}

func TestGenerateEnum(t *testing.T) {
	w := &bytes.Buffer{}
	err := GenerateProtobufDefinition(w, []interface{}{typeWithEnumField{}}, EnumMap{
		"EnumValueTwo": EnumValueTwo,
		"EnumValueOne": EnumValueOne,
	}, nil)
	assert.NoError(t, err)
	expected := `
enum EnumType {
  ENUM_VALUE_ONE = 0;
  ENUM_VALUE_TWO = 1;
}


message typeWithEnumField {
  required EnumType value = 1;
}

`
	assert.Equal(t, expected, w.String())
}
