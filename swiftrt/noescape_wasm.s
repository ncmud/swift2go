#include "textflag.h"

// func noescape(p unsafe.Pointer) unsafe.Pointer
TEXT ·noescape(SB), NOSPLIT, $0-16
	MOVD p+0(FP), R0
	MOVD R0, ret+8(FP)
	RET
