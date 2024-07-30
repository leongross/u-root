// Copyright 2012-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package memio

/*
#include <stdint.h>
// TODO: use size typed integers instead of int
int archInb(int port) {
    int value;
    __asm__ __volatile__ (
        "inb %1, %0"
        : "=a"(value)         // Output operand: value is stored in AL (part of EAX)
        : "d"(port)           // Input operand: port is specified in DX
        :                     // No clobbered registers
    );
    return value;
}
int archInw(int port) {
	int value;
	__asm__ __volatile__ (
		"inw %1, %0"
		: "=a"(value)
		: "d"(port)
		:
	);
	return value;
}
int archInl(int port) {
	int value;
	__asm__ __volatile__ (
		"inl %1, %0"
		: "=a"(value)
		: "d"(port)
		:
	);
	return value;
}
void archOutb(int port, int value) {
	__asm__ __volatile__ (
		"outb %0, %1"
		:
		: "a"(value), "d"(port)
		:
	);
}
void archOutw(int port, int value) {
	__asm__ __volatile__ (
		"outw %0, %1"
		:
		: "a"(value), "d"(port)
		:
	);
}
void archOutl(int port, int value) {
	__asm__ __volatile__ (
		"outl %0, %1"
		:
		: "a"(value), "d"(port)
		:
	);
}
*/
import "C"
