// +build amd64 amd64p

#include "textflag.h"

// func getg() *g
TEXT ·getg(SB),NOSPLIT,$0-8
	MOVQ (TLS), R14
	MOVQ R14, ret+0(FP)
	RET
