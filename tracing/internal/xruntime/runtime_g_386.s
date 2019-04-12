// +build 386

#include "textflag.h"

// func getg() *g
TEXT ·getg(SB),NOSPLIT,$0-8
	MOVL (TLS), AX
	MOVL AX, ret+0(FP)
	RET

