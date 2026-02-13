// +build arm64 loong64 mips64 mips64le ppc64 ppc64le riscv64 s390x

#include "textflag.h"

// arm, ppc, s390x:    MOVD means "move 64-bit integer", but
// riscv, mips, loong: MOVD means "move double-precision floating point"
// -> define MOV64 to cover arch differences
#ifdef GOARCH_riscv64
# define MOV64  MOV
#endif
#ifdef GOARCH_mips64
# define MOV64  MOVV
#endif
#ifdef GOARCH_mips64le
# define MOV64  MOVV
#endif
#ifdef GOARCH_loong64
# define MOV64  MOVV
#endif
#ifndef MOV64
# define MOV64  MOVD
#endif


// func getg() *g
TEXT ·getg(SB),NOSPLIT,$0-8
	MOV64 g, ret+0(FP)
	RET
