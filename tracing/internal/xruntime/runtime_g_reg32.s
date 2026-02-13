// +build arm mips mipsle

#include "textflag.h"

// func getg() *g
TEXT ·getg(SB),NOSPLIT,$0-4
	MOVW g, ret+0(FP)
	RET
