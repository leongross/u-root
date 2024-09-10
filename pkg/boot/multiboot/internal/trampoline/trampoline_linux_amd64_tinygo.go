// Copyright 2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build tinygo

package trampoline

/*
#include <stdint.h>
#include <stdio.h>
#include "textflag.h"

start:

end:

data:

info:

enrty:


magic:

uinptr_t AddrOfStart(){
	return (uintptr_t) &&start;
}

uinptr_t AddrOfEnd(){
	return (uintptr_t) &&end;
}

uinptr_t AddrOfData(){
	return (uintptr_t) &&data;
}
*/
import "C"
