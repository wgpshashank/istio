// Copyright 2019 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package env makes it possible to track use of environment variables within procress
// in order to generate documentation for these uses.
package env

import (
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"istio.io/istio/pkg/log"
)

// The type of a variable's value
type VarType byte

const (
	STRING VarType = iota
	BOOL
	INT
	FLOAT
	DURATION
)

// Var describes a single environment variable
type Var struct {
	// The name of the environment variable.
	Name string

	// The optional default value of the environment variable.
	DefaultValue string

	// Description of the environment variable's purpose.
	Description string

	// Hide the existence of this variable when outputting usage information.
	Hidden bool

	// Mark this variable as deprecated when generating usage information.
	Deprecated bool

	// The type of the variable's value
	Type VarType
}

// StringVar represents a single string environment variable.
type StringVar struct {
	Var
}

// BoolVar represents a single boolean environment variable.
type BoolVar struct {
	Var
}

// IntVar represents a single integer environment variable.
type IntVar struct {
	Var
}

// FloatVar represents a single floating-point environment variable.
type FloatVar struct {
	Var
}

// DurationVar represents a single duration environment variable.
type DurationVar struct {
	Var
}

var allVars = make(map[string]Var)
var mutex sync.Mutex

// Returns a description of this process' environment variables, sorted by name.
func VarDescriptions() []Var {
	mutex.Lock()
	sorted := make([]Var, 0, len(allVars))
	for _, v := range allVars {
		sorted = append(sorted, v)
	}
	mutex.Unlock()

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	return sorted
}

// RegisterStringVar registers a new string environment variable.
func RegisterStringVar(name string, defaultValue string, description string) StringVar {
	v := Var{Name: name, DefaultValue: defaultValue, Description: description, Type: STRING}
	RegisterVar(v)
	return StringVar{v}
}

// RegisterBoolVar registers a new boolean environment variable.
func RegisterBoolVar(name string, defaultValue bool, description string) BoolVar {
	v := Var{Name: name, DefaultValue: strconv.FormatBool(defaultValue), Description: description, Type: BOOL}
	RegisterVar(v)
	return BoolVar{v}
}

// RegisterIntVar registers a new integer environment variable.
func RegisterIntVar(name string, defaultValue int, description string) IntVar {
	v := Var{Name: name, DefaultValue: strconv.FormatInt(int64(defaultValue), 10), Description: description, Type: INT}
	RegisterVar(v)
	return IntVar{v}
}

// RegisterFloatVar registers a new floating-point environment variable.
func RegisterFloatVar(name string, defaultValue float64, description string) FloatVar {
	v := Var{Name: name, DefaultValue: strconv.FormatFloat(defaultValue, 'G', -1, 64), Description: description, Type: FLOAT}
	RegisterVar(v)
	return FloatVar{v}
}

// RegisterDurationVar registers a new duration environment variable.
func RegisterDurationVar(name string, defaultValue time.Duration, description string) DurationVar {
	v := Var{Name: name, DefaultValue: defaultValue.String(), Description: description, Type: DURATION}
	RegisterVar(v)
	return DurationVar{v}
}

// RegisterVar registers a generic environment variable.
func RegisterVar(v Var) {
	mutex.Lock()

	if old, ok := allVars[v.Name]; ok {
		if v.Description != "" {
			allVars[v.Name] = v // last one with a description wins if the same variable name is registered multiple times
		}

		if old.Description != v.Description || old.DefaultValue != v.DefaultValue || old.Type != v.Type || old.Deprecated != v.Deprecated || old.Hidden != v.Hidden {
			log.Warnf("The environment variable %s was registered multiple times using different metadata: %v, %v", v.Name, old, v)
		}
	} else {
		allVars[v.Name] = v // last one with a description wins if the same variable name is registered multiple times
	}

	mutex.Unlock()
}

func (v StringVar) Get() string {
	result, _ := v.Lookup()
	return result
}

func (v StringVar) Lookup() (string, bool) {
	result, ok := os.LookupEnv(v.Name)
	if !ok {
		result = v.DefaultValue
	}

	return result, ok
}

func (v BoolVar) Get() bool {
	result, _ := v.Lookup()
	return result
}

func (v BoolVar) Lookup() (bool, bool) {
	result, ok := os.LookupEnv(v.Name)
	if !ok {
		result = v.DefaultValue
	}

	b, err := strconv.ParseBool(result)
	if err != nil {
		log.Warnf("Invalid environment variable value `%s`, expecting true/false, defaulting to %v", result, v.DefaultValue)
		b, _ = strconv.ParseBool(v.DefaultValue)
	}

	return b, ok
}

func (v IntVar) Get() int {
	result, _ := v.Lookup()
	return result
}

func (v IntVar) Lookup() (int, bool) {
	result, ok := os.LookupEnv(v.Name)
	if !ok {
		result = v.DefaultValue
	}

	i, err := strconv.Atoi(result)
	if err != nil {
		log.Warnf("Invalid environment variable value `%s`, expecting an integer, defaulting to %v", result, v.DefaultValue)
		i, _ = strconv.Atoi(v.DefaultValue)
	}

	return i, ok
}

func (v FloatVar) Get() float64 {
	result, _ := v.Lookup()
	return result
}

func (v FloatVar) Lookup() (float64, bool) {
	result, ok := os.LookupEnv(v.Name)
	if !ok {
		result = v.DefaultValue
	}

	f, err := strconv.ParseFloat(result, 64)
	if err != nil {
		log.Warnf("Invalid environment variable value `%s`, expecting a floating-point value, defaulting to %v", result, v.DefaultValue)
		f, _ = strconv.ParseFloat(v.DefaultValue, 64)
	}

	return f, ok
}

func (v DurationVar) Get() time.Duration {
	result, _ := v.Lookup()
	return result
}

func (v DurationVar) Lookup() (time.Duration, bool) {
	result, ok := os.LookupEnv(v.Name)
	if !ok {
		result = v.DefaultValue
	}

	d, err := time.ParseDuration(result)
	if err != nil {
		log.Warnf("Invalid environment variable value `%s`, expecting a duration, defaulting to %v", result, v.DefaultValue)
		d, _ = time.ParseDuration(v.DefaultValue)
	}

	return d, ok
}
