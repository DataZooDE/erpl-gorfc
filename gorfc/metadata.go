package gorfc

/*
#include <sapnwrfc.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// FieldDescription describes one field of a structure or table row.
type FieldDescription struct {
	Name      string
	FieldType string
	NucLength uint
	NucOffset uint
	UcLength  uint
	UcOffset  uint
	Decimals  uint
	TypeDesc  TypeDescription
}

// TypeDescription describes a DDIC structure or table row type.
type TypeDescription struct {
	Name      string
	NucLength uint
	UcLength  uint
	Fields    []FieldDescription
}

// ParameterDescription describes one parameter of a function module.
type ParameterDescription struct {
	Name          string
	ParameterType string
	Direction     string
	NucLength     uint
	UcLength      uint
	Decimals      uint
	DefaultValue  string
	ParameterText string
	Optional      bool
	TypeDesc      TypeDescription
}

func (paramDesc ParameterDescription) String() string {
	return fmt.Sprintf("paramDesc(name= %v, paramType= %v, dir= %v, nucLen= %v, ucLen= %v, dec= %v, defValue= %v, paramText= %v, optional= %v, typeDesc= %v)",
		paramDesc.Name, paramDesc.ParameterType, paramDesc.Direction, paramDesc.NucLength,
		paramDesc.UcLength, paramDesc.Decimals, paramDesc.DefaultValue, paramDesc.ParameterText,
		paramDesc.Optional, paramDesc.TypeDesc)
}

// FunctionDescription describes a function module's interface.
type FunctionDescription struct {
	Name       string
	Parameters []ParameterDescription
}

func (funcDesc FunctionDescription) String() (result string) {
	result = fmt.Sprintf("FunctionDescription:\n Name: %v\n Parameters:\n", funcDesc.Name)
	for i := 0; i < len(funcDesc.Parameters); i++ {
		result += fmt.Sprintf("    %v\n", funcDesc.Parameters[i])
	}
	return
}

// GetFunctionDescription describes a function module, opening the connection
// first if it is not open.
func (conn *Connection) GetFunctionDescription(goFuncName string) (goFuncDesc FunctionDescription, err error) {
	if !conn.alive {
		if err = conn.Open(); err != nil {
			return
		}
	}

	funcName, err := fillString(goFuncName)
	if err != nil {
		return
	}
	defer C.free(unsafe.Pointer(funcName))

	var errorInfo C.RFC_ERROR_INFO
	funcDesc := C.RfcGetFunctionDesc(conn.handle, funcName, &errorInfo)
	runtime.KeepAlive(conn)
	if funcDesc == nil {
		return goFuncDesc, rfcError(errorInfo, "Could not get function description for %q", goFuncName)
	}
	return wrapFunctionDescription(funcDesc)
}

func wrapFunctionDescription(funcDesc C.RFC_FUNCTION_DESC_HANDLE) (goFuncDesc FunctionDescription, err error) {
	var errorInfo C.RFC_ERROR_INFO
	var funcName C.RFC_ABAP_NAME

	if rc := C.RfcGetFunctionName(funcDesc, &funcName[0], &errorInfo); rc != C.RFC_OK {
		return goFuncDesc, rfcError(errorInfo, "Failed getting function name")
	}
	goFuncName, err := wrapString(&funcName[0], false)
	if err != nil {
		return
	}
	goFuncDesc = FunctionDescription{Name: goFuncName}

	var paramCount C.uint
	if rc := C.RfcGetParameterCount(funcDesc, &paramCount, &errorInfo); rc != C.RFC_OK {
		return goFuncDesc, rfcError(errorInfo, "Failed getting function(%v) parameter count", goFuncName)
	}

	for i := C.uint(0); i < paramCount; i++ {
		var paramDesc C.RFC_PARAMETER_DESC
		if rc := C.RfcGetParameterDescByIndex(funcDesc, i, &paramDesc, &errorInfo); rc != C.RFC_OK {
			return goFuncDesc, rfcError(errorInfo, "Failed getting function(%v) parameter description by index(%v)", goFuncName, i)
		}

		goParamDesc := ParameterDescription{
			NucLength: uint(paramDesc.nucLength),
			UcLength:  uint(paramDesc.ucLength),
			Decimals:  uint(paramDesc.decimals),
			Optional:  paramDesc.optional != 0,
		}
		// Each of these is checked. The reference assigned five results to one
		// err and tested it once, so a failure in any but the last was lost.
		for _, step := range []struct {
			into *string
			from *C.SAP_UC
		}{
			{&goParamDesc.Name, &paramDesc.name[0]},
			{&goParamDesc.ParameterType, C.RfcGetTypeAsString(paramDesc._type)},
			{&goParamDesc.Direction, C.RfcGetDirectionAsString(paramDesc.direction)},
			{&goParamDesc.DefaultValue, &paramDesc.defaultValue[0]},
			{&goParamDesc.ParameterText, &paramDesc.parameterText[0]},
		} {
			if *step.into, err = wrapString(step.from, false); err != nil {
				return
			}
		}

		if paramDesc.typeDescHandle != nil {
			if goParamDesc.TypeDesc, err = wrapTypeDescription(paramDesc.typeDescHandle); err != nil {
				return
			}
		}
		goFuncDesc.Parameters = append(goFuncDesc.Parameters, goParamDesc)
	}
	return
}

func wrapTypeDescription(typeDesc C.RFC_TYPE_DESC_HANDLE) (goTypeDesc TypeDescription, err error) {
	var errorInfo C.RFC_ERROR_INFO
	var typeName C.RFC_ABAP_NAME

	if rc := C.RfcGetTypeName(typeDesc, &typeName[0], &errorInfo); rc != C.RFC_OK {
		return goTypeDesc, rfcError(errorInfo, "Failed getting type name")
	}
	name, err := wrapString(&typeName[0], false)
	if err != nil {
		return
	}

	var nucLength, ucLength C.uint
	if rc := C.RfcGetTypeLength(typeDesc, &nucLength, &ucLength, &errorInfo); rc != C.RFC_OK {
		return goTypeDesc, rfcError(errorInfo, "Failed getting type(%v) length", name)
	}
	goTypeDesc = TypeDescription{Name: name, NucLength: uint(nucLength), UcLength: uint(ucLength)}

	var fieldCount C.uint
	if rc := C.RfcGetFieldCount(typeDesc, &fieldCount, &errorInfo); rc != C.RFC_OK {
		return goTypeDesc, rfcError(errorInfo, "Failed getting field count")
	}

	for i := C.uint(0); i < fieldCount; i++ {
		var fieldDesc C.RFC_FIELD_DESC
		if rc := C.RfcGetFieldDescByIndex(typeDesc, i, &fieldDesc, &errorInfo); rc != C.RFC_OK {
			return goTypeDesc, rfcError(errorInfo, "Failed getting field by index(%v)", i)
		}

		goFieldDesc := FieldDescription{
			NucLength: uint(fieldDesc.nucLength),
			NucOffset: uint(fieldDesc.nucOffset),
			UcLength:  uint(fieldDesc.ucLength),
			UcOffset:  uint(fieldDesc.ucOffset),
			Decimals:  uint(fieldDesc.decimals),
		}
		if goFieldDesc.Name, err = wrapString(&fieldDesc.name[0], false); err != nil {
			return
		}
		if goFieldDesc.FieldType, err = wrapString(C.RfcGetTypeAsString(fieldDesc._type), false); err != nil {
			return
		}
		if fieldDesc.typeDescHandle != nil {
			if goFieldDesc.TypeDesc, err = wrapTypeDescription(fieldDesc.typeDescHandle); err != nil {
				return
			}
		}
		goTypeDesc.Fields = append(goTypeDesc.Fields, goFieldDesc)
	}
	return
}
