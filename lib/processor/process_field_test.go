// Copyright (c) 2018 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package processor

import (
	"reflect"
	"testing"

	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/metrics"
)

func TestProcessFieldParts(t *testing.T) {
	conf := NewConfig()
	conf.Type = "process_field"
	conf.ProcessField.Path = "foo.bar"
	conf.ProcessField.Parts = []int{1}

	procConf := NewConfig()
	procConf.Type = "json"
	procConf.JSON.Operator = "select"
	procConf.JSON.Path = "baz"

	conf.ProcessField.Processors = append(conf.ProcessField.Processors, procConf)

	c, err := New(conf, nil, log.Noop(), metrics.Noop())
	if err != nil {
		t.Error(err)
		return
	}

	exp := [][]byte{
		[]byte(`{"foo":{"bar":{"baz":"original"}}}`),
		[]byte(`{"foo":{"bar":"put me at the root"}}`),
		[]byte(`{"foo":{"bar":{"baz":"original"}}}`),
	}

	msg, res := c.ProcessMessage(message.New([][]byte{
		[]byte(`{"foo":{"bar":{"baz":"original"}}}`),
		[]byte(`{"foo":{"bar":{"baz":"put me at the root"}}}`),
		[]byte(`{"foo":{"bar":{"baz":"original"}}}`),
	}))
	if res != nil {
		t.Error(res.Error())
	}
	if act := message.GetAllBytes(msg[0]); !reflect.DeepEqual(act, exp) {
		t.Errorf("Wrong result: %s != %s", act, exp)
	}
}

func TestProcessFieldAllParts(t *testing.T) {
	conf := NewConfig()
	conf.Type = "process_field"
	conf.ProcessField.Path = "foo.bar"
	conf.ProcessField.Parts = []int{}

	procConf := NewConfig()
	procConf.Type = "json"
	procConf.JSON.Operator = "select"
	procConf.JSON.Path = "baz"

	conf.ProcessField.Processors = append(conf.ProcessField.Processors, procConf)

	c, err := New(conf, nil, log.Noop(), metrics.Noop())
	if err != nil {
		t.Error(err)
		return
	}

	exp := [][]byte{
		[]byte(`{"foo":{"bar":"put me at the root"}}`),
		[]byte(`{"foo":{"bar":"put me at the root"}}`),
	}

	msg, res := c.ProcessMessage(message.New([][]byte{
		[]byte(`{"foo":{"bar":{"baz":"put me at the root"}}}`),
		[]byte(`{"foo":{"bar":{"baz":"put me at the root"}}}`),
	}))
	if res != nil {
		t.Error(res.Error())
	}
	if act := message.GetAllBytes(msg[0]); !reflect.DeepEqual(act, exp) {
		t.Errorf("Wrong result: %s != %s", act, exp)
	}
}

func TestProcessFieldString(t *testing.T) {
	conf := NewConfig()
	conf.Type = "process_field"
	conf.ProcessField.Path = "foo.bar"
	conf.ProcessField.Parts = []int{}

	procConf := NewConfig()
	procConf.Type = "encode"

	conf.ProcessField.Processors = append(conf.ProcessField.Processors, procConf)

	c, err := New(conf, nil, log.Noop(), metrics.Noop())
	if err != nil {
		t.Error(err)
		return
	}

	exp := [][]byte{
		[]byte(`{"foo":{"bar":"ZW5jb2RlIG1l"}}`),
		[]byte(`{"foo":{"bar":"ZW5jb2RlIG1lIHRvbw=="}}`),
	}

	msg, res := c.ProcessMessage(message.New([][]byte{
		[]byte(`{"foo":{"bar":"encode me"}}`),
		[]byte(`{"foo":{"bar":"encode me too"}}`),
	}))
	if res != nil {
		t.Error(res.Error())
	}
	if act := message.GetAllBytes(msg[0]); !reflect.DeepEqual(act, exp) {
		t.Errorf("Wrong result: %s != %s", act, exp)
	}
}

func TestProcessFieldDiscard(t *testing.T) {
	conf := NewConfig()
	conf.Type = "process_field"
	conf.ProcessField.Path = "foo.bar"
	conf.ProcessField.Parts = []int{}
	conf.ProcessField.ResultType = "discard"

	procConf := NewConfig()
	procConf.Type = "encode"

	conf.ProcessField.Processors = append(conf.ProcessField.Processors, procConf)

	c, err := New(conf, nil, log.Noop(), metrics.Noop())
	if err != nil {
		t.Error(err)
		return
	}

	exp := [][]byte{
		[]byte(`{"foo":{"bar":"encode me"}}`),
		[]byte(`{"foo":{"bar":"encode me too"}}`),
	}

	msg, res := c.ProcessMessage(message.New([][]byte{
		[]byte(`{"foo":{"bar":"encode me"}}`),
		[]byte(`{"foo":{"bar":"encode me too"}}`),
	}))
	if res != nil {
		t.Error(res.Error())
	}
	if act := message.GetAllBytes(msg[0]); !reflect.DeepEqual(act, exp) {
		t.Errorf("Wrong result: %s != %s", act, exp)
	}
}

func TestProcessFieldCodecs(t *testing.T) {
	type testCase struct {
		name   string
		codec  string
		input  string
		output string
	}
	tests := []testCase{
		{
			name:   "string 1",
			codec:  "string",
			input:  `{"target":"foobar"}`,
			output: `{"target":"foobar"}`,
		},
		{
			name:   "int 1",
			codec:  "int",
			input:  `{"target":"5"}`,
			output: `{"target":5}`,
		},
		{
			name:   "float 1",
			codec:  "float",
			input:  `{"target":"5.67"}`,
			output: `{"target":5.67}`,
		},
		{
			name:   "bool 1",
			codec:  "bool",
			input:  `{"target":"true"}`,
			output: `{"target":true}`,
		},
		{
			name:   "bool 2",
			codec:  "bool",
			input:  `{"target":"false"}`,
			output: `{"target":false}`,
		},
		{
			name:   "object 1",
			codec:  "object",
			input:  `{"target":"{}"}`,
			output: `{"target":{}}`,
		},
		{
			name:   "object 2",
			codec:  "object",
			input:  `{"target":"{\"foo\":{\"bar\":\"baz\"}}"}`,
			output: `{"target":{"foo":{"bar":"baz"}}}`,
		},
		{
			name:   "object 2",
			codec:  "object",
			input:  `{"target":"null"}`,
			output: `{"target":null}`,
		},
		{
			name:   "array 1",
			codec:  "array",
			input:  `{"target":"[]"}`,
			output: `{"target":[]}`,
		},
		{
			name:   "array 2",
			codec:  "array",
			input:  `{"target":"[1,2,\"foo\"]"}`,
			output: `{"target":[1,2,"foo"]}`,
		},
	}

	procConf := NewConfig()
	procConf.Type = "noop"

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			conf := NewConfig()
			conf.Type = "process_field"
			conf.ProcessField.Path = "target"
			conf.ProcessField.ResultType = test.codec
			conf.ProcessField.Processors = append(conf.ProcessField.Processors, procConf)

			c, err := New(conf, nil, log.Noop(), metrics.Noop())
			if err != nil {
				tt.Fatal(err)
			}

			exp := [][]byte{
				[]byte(test.output),
			}
			msg, res := c.ProcessMessage(message.New([][]byte{
				[]byte(test.input),
			}))
			if res != nil {
				tt.Error(res.Error())
			}
			if act := message.GetAllBytes(msg[0]); !reflect.DeepEqual(act, exp) {
				tt.Errorf("Wrong result: %s != %s", act, exp)
			}
		})
	}
}

func TestProcessFieldBadProc(t *testing.T) {
	conf := NewConfig()
	conf.Type = "process_field"
	conf.ProcessField.Path = "foo.bar"
	conf.ProcessField.Parts = []int{}

	procConf := NewConfig()
	procConf.Type = "archive"

	conf.ProcessField.Processors = append(conf.ProcessField.Processors, procConf)

	c, err := New(conf, nil, log.Noop(), metrics.Noop())
	if err != nil {
		t.Error(err)
		return
	}

	exp := [][]byte{
		[]byte(`{"foo":{"bar":"encode me"}}`),
		[]byte(`{"foo":{"bar":"encode me too"}}`),
	}

	msg, res := c.ProcessMessage(message.New(exp))
	if res != nil {
		t.Error(res.Error())
	}
	if act := message.GetAllBytes(msg[0]); !reflect.DeepEqual(act, exp) {
		t.Errorf("Wrong result: %s != %s", act, exp)
	}
}

func TestProcessFieldBadProcTwo(t *testing.T) {
	conf := NewConfig()
	conf.Type = "process_field"
	conf.ProcessField.Path = "foo.bar"
	conf.ProcessField.Parts = []int{}

	procConf := NewConfig()
	procConf.Type = "filter"
	procConf.Filter.Type = "static"
	procConf.Filter.Static = false

	conf.ProcessField.Processors = append(conf.ProcessField.Processors, procConf)

	c, err := New(conf, nil, log.Noop(), metrics.Noop())
	if err != nil {
		t.Error(err)
		return
	}

	exp := [][]byte{
		[]byte(`{"foo":{"bar":"encode me"}}`),
		[]byte(`{"foo":{"bar":"encode me too"}}`),
	}

	msg, res := c.ProcessMessage(message.New(exp))
	if res != nil {
		t.Error(res.Error())
	}
	if act := message.GetAllBytes(msg[0]); !reflect.DeepEqual(act, exp) {
		t.Errorf("Wrong result: %s != %s", act, exp)
	}
}
