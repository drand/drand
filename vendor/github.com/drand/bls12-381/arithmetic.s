#include "textflag.h"


// single-precision addition w/ modular reduction
// a' = (a + b) % p
TEXT ·add_assign_6(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13

	// |
	ADDQ (SI), R8
	ADCQ 8(SI), R9
	ADCQ 16(SI), R10
	ADCQ 24(SI), R11
	ADCQ 32(SI), R12
	ADCQ 40(SI), R13

	// |
	MOVQ R8, R14
	MOVQ R9, R15
	MOVQ R10, CX
	MOVQ R11, DX
	MOVQ R12, SI
	MOVQ R13, BX
	MOVQ $0xb9feffffffffaaab, AX
	SUBQ AX, R14
	MOVQ $0x1eabfffeb153ffff, AX
	SBBQ AX, R15
	MOVQ $0x6730d2a0f6b0f624, AX
	SBBQ AX, CX
	MOVQ $0x64774b84f38512bf, AX
	SBBQ AX, DX
	MOVQ $0x4b1ba7b6434bacd7, AX
	SBBQ AX, SI
	MOVQ $0x1a0111ea397fe69a, AX
	SBBQ AX, BX
	CMOVQCC R14, R8
	CMOVQCC R15, R9
	CMOVQCC CX, R10
	CMOVQCC DX, R11
	CMOVQCC SI, R12
	CMOVQCC BX, R13

	// |
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET

/*	 | end add6alt									*/


// single-precision addition w/ modular reduction
// c = (a + b) % p
TEXT ·add6(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13

	// |
	ADDQ (SI), R8
	ADCQ 8(SI), R9
	ADCQ 16(SI), R10
	ADCQ 24(SI), R11
	ADCQ 32(SI), R12
	ADCQ 40(SI), R13

	// |
	MOVQ R8, R14
	MOVQ R9, R15
	MOVQ R10, CX
	MOVQ R11, DX
	MOVQ R12, SI
	MOVQ R13, BX
	MOVQ $0xb9feffffffffaaab, DI
	SUBQ DI, R14
	MOVQ $0x1eabfffeb153ffff, DI
	SBBQ DI, R15
	MOVQ $0x6730d2a0f6b0f624, DI
	SBBQ DI, CX
	MOVQ $0x64774b84f38512bf, DI
	SBBQ DI, DX
	MOVQ $0x4b1ba7b6434bacd7, DI
	SBBQ DI, SI
	MOVQ $0x1a0111ea397fe69a, DI
	SBBQ DI, BX
	CMOVQCC R14, R8
	CMOVQCC R15, R9
	CMOVQCC CX, R10
	CMOVQCC DX, R11
	CMOVQCC SI, R12
	CMOVQCC BX, R13

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// single-precision addition w/o reduction check
// c = (a + b)
TEXT ·ladd6(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13

	// |
	ADDQ (SI), R8
	ADCQ 8(SI), R9
	ADCQ 16(SI), R10
	ADCQ 24(SI), R11
	ADCQ 32(SI), R12
	ADCQ 40(SI), R13

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// single-precision addition w/o check
// a' = a + b
TEXT ·ladd_assign_6(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13

	// |
	ADDQ (SI), R8
	ADCQ 8(SI), R9
	ADCQ 16(SI), R10
	ADCQ 24(SI), R11
	ADCQ 32(SI), R12
	ADCQ 40(SI), R13

	// |
	MOVQ a+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// double-precision addition w/o upper bound check
// c = a + b
TEXT ·ladd12(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	ADDQ (SI), R8
	ADCQ 8(SI), R9
	ADCQ 16(SI), R10
	ADCQ 24(SI), R11
	ADCQ 32(SI), R12
	ADCQ 40(SI), R13
	ADCQ 48(SI), R14
	ADCQ 56(SI), R15
	ADCQ 64(SI), AX
	ADCQ 72(SI), BX
	ADCQ 80(SI), CX
	ADCQ 88(SI), DX

	// |
	MOVQ c+0(FP), SI
	MOVQ R8, (SI)
	MOVQ R9, 8(SI)
	MOVQ R10, 16(SI)
	MOVQ R11, 24(SI)
	MOVQ R12, 32(SI)
	MOVQ R13, 40(SI)
	MOVQ R14, 48(SI)
	MOVQ R15, 56(SI)
	MOVQ AX, 64(SI)
	MOVQ BX, 72(SI)
	MOVQ CX, 80(SI)
	MOVQ DX, 88(SI)
	RET
/*	 | end													*/


// double-precision addition w/ upper bound check
// if c > (2^N)p , 
// then correct by c = c - (2^N)p
// c = a + b
TEXT ·add12(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	ADDQ (SI), R8
	ADCQ 8(SI), R9
	ADCQ 16(SI), R10
	ADCQ 24(SI), R11
	ADCQ 32(SI), R12
	ADCQ 40(SI), R13
	ADCQ 48(SI), R14
	ADCQ 56(SI), R15
	ADCQ 64(SI), AX
	ADCQ 72(SI), BX
	ADCQ 80(SI), CX
	ADCQ 88(SI), DX

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)


	MOVQ R14, R8
	MOVQ R15, R9
	MOVQ AX, R10
	MOVQ BX, R11
	MOVQ CX, R12
	MOVQ DX, R13
	MOVQ $0xb9feffffffffaaab, SI
	SUBQ SI, R8
	MOVQ $0x1eabfffeb153ffff, SI
	SBBQ SI, R9
	MOVQ $0x6730d2a0f6b0f624, SI
	SBBQ SI, R10
	MOVQ $0x64774b84f38512bf, SI
	SBBQ SI, R11
	MOVQ $0x4b1ba7b6434bacd7, SI
	SBBQ SI, R12
	MOVQ $0x1a0111ea397fe69a, SI
	SBBQ SI, R13
	CMOVQCC R8, R14
	CMOVQCC R9, R15
	CMOVQCC R10, AX
	CMOVQCC R11, BX
	CMOVQCC R12, CX
	CMOVQCC R13, DX

	// |
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// double-precision addition w/ upper bound check
// if a' > (2^N)p , 
// then correct by a' = a' - (2^N)p
// a' = a * b
TEXT ·add_assign_12(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	ADDQ (SI), R8
	ADCQ 8(SI), R9
	ADCQ 16(SI), R10
	ADCQ 24(SI), R11
	ADCQ 32(SI), R12
	ADCQ 40(SI), R13
	ADCQ 48(SI), R14
	ADCQ 56(SI), R15
	ADCQ 64(SI), AX
	ADCQ 72(SI), BX
	ADCQ 80(SI), CX
	ADCQ 88(SI), DX

	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)

	MOVQ R14, R8
	MOVQ R15, R9
	MOVQ AX, R10
	MOVQ BX, R11
	MOVQ CX, R12
	MOVQ DX, R13
	MOVQ $0xb9feffffffffaaab, SI
	SUBQ SI, R8
	MOVQ $0x1eabfffeb153ffff, SI
	SBBQ SI, R9
	MOVQ $0x6730d2a0f6b0f624, SI
	SBBQ SI, R10
	MOVQ $0x64774b84f38512bf, SI
	SBBQ SI, R11
	MOVQ $0x4b1ba7b6434bacd7, SI
	SBBQ SI, R12
	MOVQ $0x1a0111ea397fe69a, SI
	SBBQ SI, R13
	CMOVQCC R8, R14
	CMOVQCC R9, R15
	CMOVQCC R10, AX
	CMOVQCC R11, BX
	CMOVQCC R12, CX
	CMOVQCC R13, DX

	// |
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// single-precision subtraction with modular reduction
// c = (a - b) % p
TEXT ·sub6(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI
	XORQ AX, AX

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13

	// |
	MOVQ $0xb9feffffffffaaab, R14
	MOVQ $0x1eabfffeb153ffff, R15
	MOVQ $0x6730d2a0f6b0f624, CX
	MOVQ $0x64774b84f38512bf, DX
	MOVQ $0x4b1ba7b6434bacd7, SI
	MOVQ $0x1a0111ea397fe69a, BX
	CMOVQCC AX, R14
	CMOVQCC AX, R15
	CMOVQCC AX, CX
	CMOVQCC AX, DX
	CMOVQCC AX, SI
	CMOVQCC AX, BX
	ADDQ R14, R8
	ADCQ R15, R9
	ADCQ CX, R10
	ADCQ DX, R11
	ADCQ SI, R12
	ADCQ BX, R13

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// single-precision subtraction with modular reduction
// a' = (a - b) % p
TEXT ·sub_assign_6(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI
	XORQ AX, AX

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13

	// |
	MOVQ $0xb9feffffffffaaab, R14
	MOVQ $0x1eabfffeb153ffff, R15
	MOVQ $0x6730d2a0f6b0f624, CX
	MOVQ $0x64774b84f38512bf, DX
	MOVQ $0x4b1ba7b6434bacd7, SI
	MOVQ $0x1a0111ea397fe69a, BX
	CMOVQCC AX, R14
	CMOVQCC AX, R15
	CMOVQCC AX, CX
	CMOVQCC AX, DX
	CMOVQCC AX, SI
	CMOVQCC AX, BX
	ADDQ R14, R8
	ADCQ R15, R9
	ADCQ CX, R10
	ADCQ DX, R11
	ADCQ SI, R12
	ADCQ BX, R13

	// |
	MOVQ a+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// single-precision subtraction plus modulus
// c = (a - b) + p
TEXT ·lsub6(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13

	// |
	MOVQ $0xb9feffffffffaaab, R14
	MOVQ $0x1eabfffeb153ffff, R15
	MOVQ $0x6730d2a0f6b0f624, CX
	MOVQ $0x64774b84f38512bf, DX
	MOVQ $0x4b1ba7b6434bacd7, SI
	MOVQ $0x1a0111ea397fe69a, BX
	ADDQ R14, R8
	ADCQ R15, R9
	ADCQ CX, R10
	ADCQ DX, R11
	ADCQ SI, R12
	ADCQ BX, R13

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// single-precision subtraction plus modulus
// a' = (a - b)
TEXT ·lsub_assign_nc_6(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13
	
	// |
	MOVQ a+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// double-precision subtraction w/o check
// c = (a - b)
TEXT ·lsub12(SB), NOSPLIT, $0-24

	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13
	SBBQ 48(SI), R14
	SBBQ 56(SI), R15
	SBBQ 64(SI), AX
	SBBQ 72(SI), BX
	SBBQ 80(SI), CX
	SBBQ 88(SI), DX

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/

// double-precision subtraction w/o check
// c = (a - b)
TEXT ·lsub_assign_12(SB), NOSPLIT, $0-16

	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13
	SBBQ 48(SI), R14
	SBBQ 56(SI), R15
	SBBQ 64(SI), AX
	SBBQ 72(SI), BX
	SBBQ 80(SI), CX
	SBBQ 88(SI), DX

	// |
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// double-precision subtraction.
// [AKLGL] Option2
// https://eprint.iacr.org/2010/526 
// c = (a - b)
// if c is negative, 
// then correct by c = c + (2^N)p
TEXT ·sub12(SB), NOSPLIT, $0-24

	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX


	// |
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13
	SBBQ 48(SI), R14
	SBBQ 56(SI), R15
	SBBQ 64(SI), AX
	SBBQ 72(SI), BX
	SBBQ 80(SI), CX
	SBBQ 88(SI), DX

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)

 	MOVQ $0, SI
	// |
	MOVQ $0xb9feffffffffaaab, R8
	MOVQ $0x1eabfffeb153ffff, R9
	MOVQ $0x6730d2a0f6b0f624, R10
	MOVQ $0x64774b84f38512bf, R11
	MOVQ $0x4b1ba7b6434bacd7, R12
	MOVQ $0x1a0111ea397fe69a, R13
	CMOVQCC SI, R8
	CMOVQCC SI, R9
	CMOVQCC SI, R10
	CMOVQCC SI, R11
	CMOVQCC SI, R12
	CMOVQCC SI, R13
	ADDQ R8, R14
	ADCQ R9, R15
	ADCQ R10, AX
	ADCQ R11, BX
	ADCQ R12, CX
	ADCQ R13, DX

	// |
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// double-precision subtraction.
// [AKLGL] Option2
// https://eprint.iacr.org/2010/526 
// c = (a - b)
// if c is negative, 
// then correct by c = c + (2^N)p
TEXT ·sub_assign_12(SB), NOSPLIT, $0-16

	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX


	// |
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13
	SBBQ 48(SI), R14
	SBBQ 56(SI), R15
	SBBQ 64(SI), AX
	SBBQ 72(SI), BX
	SBBQ 80(SI), CX
	SBBQ 88(SI), DX

	// |
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)

 	MOVQ $0, SI
	// |
	MOVQ $0xb9feffffffffaaab, R8
	MOVQ $0x1eabfffeb153ffff, R9
	MOVQ $0x6730d2a0f6b0f624, R10
	MOVQ $0x64774b84f38512bf, R11
	MOVQ $0x4b1ba7b6434bacd7, R12
	MOVQ $0x1a0111ea397fe69a, R13
	CMOVQCC SI, R8
	CMOVQCC SI, R9
	CMOVQCC SI, R10
	CMOVQCC SI, R11
	CMOVQCC SI, R12
	CMOVQCC SI, R13
	ADDQ R8, R14
	ADCQ R9, R15
	ADCQ R10, AX
	ADCQ R11, BX
	ADCQ R12, CX
	ADCQ R13, DX

	// |
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// [AKLGL] double-precision subtraction, option 1. h = 2
// https://eprint.iacr.org/2010/526
// c = (a - b) + p*2^(N-2)
TEXT ·sub12_opt1_h2(SB), NOSPLIT, $0-24

	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13
	SBBQ 48(SI), R14
	SBBQ 56(SI), R15
	SBBQ 64(SI), AX
	SBBQ 72(SI), BX
	SBBQ 80(SI), CX
	SBBQ 88(SI), DX

	// |
	ADDQ ·h2+0(SB), R13
	ADCQ ·h2+8(SB), R14
	ADCQ ·h2+16(SB), R15
	ADCQ ·h2+24(SB), AX
	ADCQ ·h2+32(SB), BX
	ADCQ ·h2+40(SB), CX
	ADCQ ·h2+48(SB), DX

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// [AKLGL] double-precision subtraction, option 1. h = 1
// https://eprint.iacr.org/2010/526
// c = (a - b) + p*2^(N-1)
TEXT ·sub12_opt1_h1(SB), NOSPLIT, $0-24

	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	SUBQ (SI), R8
	SBBQ 8(SI), R9
	SBBQ 16(SI), R10
	SBBQ 24(SI), R11
	SBBQ 32(SI), R12
	SBBQ 40(SI), R13
	SBBQ 48(SI), R14
	SBBQ 56(SI), R15
	SBBQ 64(SI), AX
	SBBQ 72(SI), BX
	SBBQ 80(SI), CX
	SBBQ 88(SI), DX

	// |
	ADDQ ·h1+0(SB), R13
	ADCQ ·h1+8(SB), R14
	ADCQ ·h1+16(SB), R15
	ADCQ ·h1+24(SB), AX
	ADCQ ·h1+32(SB), BX
	ADCQ ·h1+40(SB), CX
	ADCQ ·h1+48(SB), DX

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// single-precision doubling
// c = (2 * a) % p
TEXT ·double6(SB), NOSPLIT, $0-16
	// |
	MOVQ a+8(FP), DI

	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	ADDQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13

	// |
	MOVQ R8, R14
	MOVQ R9, R15
	MOVQ R10, CX
	MOVQ R11, DX
	MOVQ R12, SI
	MOVQ R13, BX
	MOVQ $0xb9feffffffffaaab, DI
	SUBQ DI, R14
	MOVQ $0x1eabfffeb153ffff, DI
	SBBQ DI, R15
	MOVQ $0x6730d2a0f6b0f624, DI
	SBBQ DI, CX
	MOVQ $0x64774b84f38512bf, DI
	SBBQ DI, DX
	MOVQ $0x4b1ba7b6434bacd7, DI
	SBBQ DI, SI
	MOVQ $0x1a0111ea397fe69a, DI
	SBBQ DI, BX
	CMOVQCC R14, R8
	CMOVQCC R15, R9
	CMOVQCC CX, R10
	CMOVQCC DX, R11
	CMOVQCC SI, R12
	CMOVQCC BX, R13

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// single-precision doubling
// a' = (2 * a) % p
TEXT ·double_assign_6(SB), NOSPLIT, $0-8
	// |
	MOVQ a+0(FP), DI

	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	ADDQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13

	// |
	MOVQ R8, R14
	MOVQ R9, R15
	MOVQ R10, CX
	MOVQ R11, DX
	MOVQ R12, SI
	MOVQ R13, BX
	MOVQ $0xb9feffffffffaaab, AX
	SUBQ AX, R14
	MOVQ $0x1eabfffeb153ffff, AX
	SBBQ AX, R15
	MOVQ $0x6730d2a0f6b0f624, AX
	SBBQ AX, CX
	MOVQ $0x64774b84f38512bf, AX
	SBBQ AX, DX
	MOVQ $0x4b1ba7b6434bacd7, AX
	SBBQ AX, SI
	MOVQ $0x1a0111ea397fe69a, AX
	SBBQ AX, BX
	CMOVQCC R14, R8
	CMOVQCC R15, R9
	CMOVQCC CX, R10
	CMOVQCC DX, R11
	CMOVQCC SI, R12
	CMOVQCC BX, R13

	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


// single-precision doubling w/o carry check
// c = 2 * a
TEXT ·ldouble6(SB), NOSPLIT, $0-16
	// |
	MOVQ a+8(FP), DI

	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13

	// |
	ADDQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)

	RET
/*	 | end													*/


// double-precision doubling w/ upper bound check
// if c > (2^N)p , 
// then correct by c = c - (2^N)p
// c = 2 * a
TEXT ·double12(SB), NOSPLIT, $0-16
	// |
	MOVQ a+8(FP), DI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	ADDQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13
	ADCQ R14, R14
	ADCQ R15, R15
	ADCQ AX, AX
	ADCQ BX, BX
	ADCQ CX, CX
	ADCQ DX, DX

	// |
	MOVQ c+0(FP), SI
	MOVQ R8, (SI)
	MOVQ R9, 8(SI)
	MOVQ R10, 16(SI)
	MOVQ R11, 24(SI)
	MOVQ R12, 32(SI)
	MOVQ R13, 40(SI)

	// |
	MOVQ R14, R8
	MOVQ R15, R9
	MOVQ AX, R10
	MOVQ BX, R11
	MOVQ CX, R12
	MOVQ DX, R13
	MOVQ $0xb9feffffffffaaab, DI
	SUBQ DI, R8
	MOVQ $0x1eabfffeb153ffff, DI
	SBBQ DI, R9
	MOVQ $0x6730d2a0f6b0f624, DI
	SBBQ DI, R10
	MOVQ $0x64774b84f38512bf, DI
	SBBQ DI, R11
	MOVQ $0x4b1ba7b6434bacd7, DI
	SBBQ DI, R12
	MOVQ $0x1a0111ea397fe69a, DI
	SBBQ DI, R13
	CMOVQCC R8, R14
	CMOVQCC R9, R15
	CMOVQCC R10, AX
	CMOVQCC R11, BX
	CMOVQCC R12, CX
	CMOVQCC R13, DX

	// |
	MOVQ R14, 48(SI)
	MOVQ R15, 56(SI)
	MOVQ AX, 64(SI)
	MOVQ BX, 72(SI)
	MOVQ CX, 80(SI)
	MOVQ DX, 88(SI)
	RET
/*	 | end													*/


// double-precision doubling w/ upper bound check
// if a' > (2^N)p , 
// then correct by a' = a' - (2^N)p
// a' = 2 * a
TEXT ·double_assign_12(SB), NOSPLIT, $0-8
	// |
	MOVQ a+0(FP), DI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	ADDQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13
	ADCQ R14, R14
	ADCQ R15, R15
	ADCQ AX, AX
	ADCQ BX, BX
	ADCQ CX, CX
	ADCQ DX, DX

	// |
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)

	// |
	MOVQ R14, R8
	MOVQ R15, R9
	MOVQ AX, R10
	MOVQ BX, R11
	MOVQ CX, R12
	MOVQ DX, R13
	MOVQ $0xb9feffffffffaaab, SI
	SUBQ SI, R8
	MOVQ $0x1eabfffeb153ffff, SI
	SBBQ SI, R9
	MOVQ $0x6730d2a0f6b0f624, SI
	SBBQ SI, R10
	MOVQ $0x64774b84f38512bf, SI
	SBBQ SI, R11
	MOVQ $0x4b1ba7b6434bacd7, SI
	SBBQ SI, R12
	MOVQ $0x1a0111ea397fe69a, SI
	SBBQ SI, R13
	CMOVQCC R8, R14
	CMOVQCC R9, R15
	CMOVQCC R10, AX
	CMOVQCC R11, BX
	CMOVQCC R12, CX
	CMOVQCC R13, DX

	// |
	MOVQ R14, 48(DI)
	MOVQ R15, 56(DI)
	MOVQ AX, 64(DI)
	MOVQ BX, 72(DI)
	MOVQ CX, 80(DI)
	MOVQ DX, 88(DI)
	RET
/*	 | end													*/


// double-precision doubling w/o upper bound check
// c = 2 * a
TEXT ·ldouble12(SB), NOSPLIT, $0-16
	// |
	MOVQ a+8(FP), DI

	// |
	MOVQ (DI), R8
	MOVQ 8(DI), R9
	MOVQ 16(DI), R10
	MOVQ 24(DI), R11
	MOVQ 32(DI), R12
	MOVQ 40(DI), R13
	MOVQ 48(DI), R14
	MOVQ 56(DI), R15
	MOVQ 64(DI), AX
	MOVQ 72(DI), BX
	MOVQ 80(DI), CX
	MOVQ 88(DI), DX

	// |
	ADDQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13
	ADCQ R14, R14
	ADCQ R15, R15
	ADCQ AX, AX
	ADCQ BX, BX
	ADCQ CX, CX
	ADCQ DX, DX

	// |
	MOVQ c+0(FP), SI
	MOVQ R8, (SI)
	MOVQ R9, 8(SI)
	MOVQ R10, 16(SI)
	MOVQ R11, 24(SI)
	MOVQ R12, 32(SI)
	MOVQ R13, 40(SI)
	MOVQ R14, 48(SI)
	MOVQ R15, 56(SI)
	MOVQ AX, 64(SI)
	MOVQ BX, 72(SI)
	MOVQ CX, 80(SI)
	MOVQ DX, 88(SI)
	RET
/*	 | end													*/


TEXT ·neg(SB), NOSPLIT, $0-16
	// |
	MOVQ a+8(FP), DI

	// |
	MOVQ $0xb9feffffffffaaab, R8
	MOVQ $0x1eabfffeb153ffff, R9
	MOVQ $0x6730d2a0f6b0f624, R10
	MOVQ $0x64774b84f38512bf, R11
	MOVQ $0x4b1ba7b6434bacd7, R12
	MOVQ $0x1a0111ea397fe69a, R13
	SUBQ (DI), R8
	SBBQ 8(DI), R9
	SBBQ 16(DI), R10
	SBBQ 24(DI), R11
	SBBQ 32(DI), R12
	SBBQ 40(DI), R13

	// |
	MOVQ c+0(FP), DI
	MOVQ R8, (DI)
	MOVQ R9, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R11, 24(DI)
	MOVQ R12, 32(DI)
	MOVQ R13, 40(DI)
	RET
/*	 | end													*/


TEXT ·mul_nobmi2(SB), NOSPLIT, $0-24

/*	 | inputs												*/

	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI
	MOVQ c+0(FP), R15

	// | w = a * b
	// | a = (a0, a1, a2, a3, a4, a5)
	// | b = (b0, b1, b2, b3, b4, b5)
	// | w = (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11)

	// | notice that:
	// | a5 and b5 is lower than 0x1a0111ea397fe69a
	// | high(0x1a0111ea397fe69a * 0xffffffffffffffff) 
	// |  = 1a0111ea397fe699

	MOVQ $0, R9
	MOVQ $0, R10
	MOVQ $0, R11
	MOVQ $0, R12
	MOVQ $0, R13
	MOVQ $0, R14
	MOVQ $0, BX

/*	 | i0														*/

	// | b0 @ CX
	MOVQ (SI), CX

	// | a0 * b0
	// | (w0, w1) @ (ret, R8)
	MOVQ (DI), AX
	MULQ CX
	MOVQ AX, 0(R15)
	MOVQ DX, R8

	// | a1 * b0
	// | (w1, w2) @ (R8, R9)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | a2 * b0
	// | (w2, w3) @ (R9, R10)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | a3 * b0
	// | (w3, w4) @ (R10, R11)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11

	// | a4 * b0
	// | (w4, w5) @ (R11, R12)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12

	// | a5 * b0
	// | (w5, w6) @ (R12, R13)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | ret,  R8,  R9, R10, R11, R12, R13,   -,  -,   -,   -,   -

/*	 | i1														*/

	// | b1 @ CX
	MOVQ 8(SI), CX

	// | a0 * b1
	// | (w1, w2, w3, w4) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w1 @ ret
	MOVQ R8, 8(R15)

	// | a1 * b1
	// | (w2, w3, w4, w5) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX

	// | a2 * b1
	// | (w3, w4, w5, w6) @ (R10, R11, R12, R13)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	ADCQ $0, R13

	// | a3 * b1
	// | (w4, w5, w6) @ (R11, R12, R13)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ $0, R13

	// | a4 * b1
	// | (w5, w6, w7) @ (R12, R13, R14)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	MOVQ $0, R14
	ADCQ $0, R14

	// | a5 * b1
	// | (w6, w7) @ (R13, R14)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | ret, ret,  R9, R10, R11, R12, R13, R14,  -,   -,   -,   -

/*	 | i2														*/

	// | b2 @ CX
	MOVQ 16(SI), CX
	MOVQ $0, BX

	// | a0 * b2
	// | (w2, w3, w4, w5) @ (R9, R10, R11, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ $0, R11
	ADCQ $0, BX
	// | w2 @ ret
	MOVQ R9, 16(R15)

	// | a1 * b2
	// | (w3, w4, w5, w6) @ (R10, R11, R12, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX

	// | a2 * b2
	// | (w4, w5, w6, w7) @ (R11, R12, R13, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	ADCQ $0, R14

	// | a3 * b2
	// | (w5, w6, w7, w8) @ (R12, R13, R14)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, R14

	// | a4 * b2
	// | (w6, w7, w8) @ (R13, R14, R8)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	MOVQ $0, R8
	ADCQ $0, R8

	// | a5 * b1
	// | (w7, w8) @ (R14, R8)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R8

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | ret, ret, ret, R10, R11, R12, R13, R14,  R8,   -,   -,   -

/*	 | i3														*/

	// | b3 @ CX
	MOVQ 24(SI), CX
	MOVQ $0, BX

	// | a0 * b3
	// | (w3, w4, w5, w6) @ (R10, R11, R12, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ $0, R12
	ADCQ $0, BX
	// | w3 @ ret
	MOVQ R10, 24(R15)

	// | a1 * b3
	// | (w4, w5, w6, w7) @ (R11, R12, R13, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX

	// | a2 * b3
	// | (w5, w6, w7, w8) @ (R12, R13, R14, R8)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	ADCQ $0, R8

	// | a3 * b3
	// | (w6, w7, w8, w9) @ (R13, R14, R8)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ $0, R8

	// | a4 * b3
	// | (w7, w8, w9) @ (R14, R8, R9)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R8
	MOVQ $0, R9
	ADCQ $0, R9

	// | a5 * b3
	// | (w8, w9) @ (R8, R9)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | ret, ret, ret, ret, R11, R12, R13, R14,  R8,  R9,   -,   -

/*	 | i4														*/

	// | b4 @ CX
	MOVQ 32(SI), CX
	MOVQ $0, BX

	// | a0 * b4
	// | (w4, w5, w6, w7) @ (R11, R12, R13, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ $0, R13
	ADCQ $0, BX
	// | w4 @ ret
	MOVQ R11, 32(R15)

	// | a1 * b4
	// | (w5, w6, w7, w8) @ (R12, R13, R14, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0, BX
	ADCQ $0, BX

	// | a2 * b4
	// | (w6, w7, w8, w9) @ (R13, R14, R8, R9)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R8
	ADCQ $0, R9

	// | a3 * b4
	// | (w7, w8, w9, w10) @ (R14, R8, R9, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R8
	ADCQ $0, R9

	// | a4 * b4
	// | (w8, w9, w10) @ (R8, R9, R10)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	MOVQ $0, R10
	ADCQ $0, R10

	// | a5 * b4
	// | (w9, w10) @ (R9, R10)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | ret, ret, ret, ret, ret, R12, R13, R14,  R8,  R9, R10,   -

/*	 | i5														*/

		// | b5 @ CX
	MOVQ 40(SI), CX
	MOVQ $0, BX

	// | a0 * b5
	// | (w5, w6, w7, w8) @ (R12, R13, R14, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, R14
	ADCQ $0, BX

	// | a1 * b5
	// | (w6, w7, w8, w9) @ (R13, R14, R8, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R8
	MOVQ $0, BX
	ADCQ $0, BX

	// | a2 * b5
	// | (w7, w8, w9, w10) @ (R14, R8, R9, R10)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R8
	ADCQ BX, R9
	ADCQ $0, R10

	// | a3 * b5
	// | (w8, w9, w10, w11) @ (R8, R9, R10)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10

	// | a4 * b5
	// | (w9, w10, w11) @ (R9, R10, R11)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | a5 * b5
	// | (w10, w11) @ (R10, R11)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ $0, DX

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | ret, ret, ret, ret, ret, R12, R13, R14,  R8,  R9, R10,  DX
/*	 | ret								 					*/
	MOVQ R12, 40(R15)
	MOVQ R13, 48(R15)
	MOVQ R14, 56(R15)
	MOVQ R8, 64(R15)
	MOVQ R9, 72(R15)
	MOVQ R10, 80(R15)
	MOVQ DX, 88(R15)

	RET
/*	 | end								 					*/	


TEXT ·mont_nobmi2(SB), NOSPLIT, $0-16

/*	 | inputs												*/

	MOVQ a+8(FP), SI
	MOVQ 8(SI), R8
	MOVQ 16(SI), R9
	MOVQ 24(SI), R10
	MOVQ 32(SI), R11
	MOVQ 40(SI), R12
	MOVQ 48(SI), R13
	MOVQ 56(SI), R14
	MOVQ 64(SI), R15

/*	 | i0														*/

	// | (u @ BX) = (w0 @ 0(SI)) * inverse_p
	MOVQ 0(SI), AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX

	MOVQ $0, CX
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, CX

	// | w1 @ R8
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ CX, R8
	MOVQ $0, CX
	ADCQ DX, CX

	// | w2 @ R9
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ CX, R9
	MOVQ $0, CX
	ADCQ DX, CX

	// | w3 @ R10
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ CX, R10
	MOVQ $0, CX
	ADCQ DX, CX

	// | w4 @ R11
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ CX, R11
	MOVQ $0, CX
	ADCQ DX, CX

	// | w5 @ R12
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ $0, DX
	ADDQ CX, R12
	// | w6 @ R13
	ADCQ DX, R13
	
	// | long_carry @ DI should be added to w7
	MOVQ $0, DI
	ADCQ $0, DI

	// |  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R8,  R9, R10, R11, R12, R13, R14,   -,   -,   -,   -

/*	 | i1														*/

		// | (u @ BX) = (w1 @ R8) * inverse_p
	MOVQ R8, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX

	MOVQ $0, CX
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, CX
	// | R8 is idle now

	// | w2 @ R9
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ CX, R9
	MOVQ $0, CX
	ADCQ DX, CX

	// | w3 @ R10
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ CX, R10
	MOVQ $0, CX
	ADCQ DX, CX

	// | w4 @ R11
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ CX, R11
	MOVQ $0, CX
	ADCQ DX, CX

	// | w5 @ R12
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ $0, DX
	ADDQ CX, R12
	MOVQ $0, CX
	ADCQ DX, CX

	// | w6 @ R13
	// | in the last round of the iteration
	// | we don't use the short carry @ CX
	// | instead we bring back long_carry @ DI
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R13
	ADCQ DX, DI
	ADDQ CX, R13
	// | w7 @ R14
	ADCQ DI, R14
	// | long_carry @ DI should be added to w8
	MOVQ $0, DI
	ADCQ $0, DI

	// |   -,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R8,  R9, R10, R11, R12, R13, R14,   -,   -,   -,   -

/*	 | i2														*/

		// | (u @ BX) = (w2 @ R9) * inverse_p
	MOVQ R9, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX

	MOVQ $0, CX
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, CX
	// | R9 is idle now

	// | w3 @ R10
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ CX, R10
	MOVQ $0, CX
	ADCQ DX, CX

	// | w4 @ R11
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ CX, R11
	MOVQ $0, CX
	ADCQ DX, CX

	// | w5 @ R12
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ $0, DX
	ADDQ CX, R12
	MOVQ $0, CX
	ADCQ DX, CX

	// | w6 @ R13
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R13
	ADCQ $0, DX
	ADDQ CX, R13
	MOVQ $0, CX
	ADCQ DX, CX

	// | w7 @ R14
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ DX, DI
	ADDQ CX, R14
	// | w8 @ R15
	ADCQ DI, R15
	// | long_carry @ DI should be added to w8
	MOVQ $0, DI
	ADCQ $0, DI

	// |   -,   -,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R8,  R9, R10, R11, R12, R13, R14, R15,   -,   -,   -

/*	 | i3														*/

	// | w9 @ R8
	MOVQ 72(SI), R8

		// | (u @ BX) = (w3 @ R10) * inverse_p
	MOVQ R10, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX

	MOVQ $0, CX
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, CX
	// | R10 is idle now

	// | w4 @ R11
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ CX, R11
	MOVQ $0, CX
	ADCQ DX, CX

	// | w5 @ R12
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ $0, DX
	ADDQ CX, R12
	MOVQ $0, CX
	ADCQ DX, CX

	// | w6 @ R13
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R13
	ADCQ $0, DX
	ADDQ CX, R13
	MOVQ $0, CX
	ADCQ DX, CX

	// | w7 @ R14
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ CX, R14
	MOVQ $0, CX
	ADCQ DX, CX

	// | w8 @ R15
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ DX, DI
	ADDQ CX, R15
	// | w9 @ R8
	ADCQ DI, R8
	// | long_carry @ DI should be added to w8
	MOVQ $0, DI
	ADCQ $0, DI

	// |   -,   -,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R9, R10, R11, R12, R13, R14, R15,  R8,   -,   -

/*	 | i4														*/

	// | w10 @ R9
	MOVQ 80(SI), R9

	// | (u @ BX) = (w4 @ R11) * inverse_p
	MOVQ R11, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX

	MOVQ $0, CX
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, CX
	// | R11 is idle now

	// | w5 @ R12
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ $0, DX
	ADDQ CX, R12
	MOVQ $0, CX
	ADCQ DX, CX

	// | w6 @ R13
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R13
	ADCQ $0, DX
	ADDQ CX, R13
	MOVQ $0, CX
	ADCQ DX, CX

	// | w7 @ R14
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ CX, R14
	MOVQ $0, CX
	ADCQ DX, CX

	// | w8 @ R15
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ CX, R15
	MOVQ $0, CX
	ADCQ DX, CX

	// | w9 @ R8
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ DX, DI
	ADDQ CX, R8
	// | w10 @ R9
	ADCQ DI, R9
	// | long_carry @ DI should be added to w8
	MOVQ $0, DI
	ADCQ $0, DI

	// |   -,   -,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | R10, R11, R12, R13, R14, R15,  R8,  R9,   -

/*	 | i5														*/

	// | w11 @ R10
	MOVQ 88(SI), R10

	// | (u @ BX) = (w5 @ R12) * inverse_p
	MOVQ R12, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX

	MOVQ $0, CX
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, CX

	// | w6 @ R13
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R13
	ADCQ $0, DX
	ADDQ CX, R13
	MOVQ $0, CX
	ADCQ DX, CX

	// | w7 @ R14
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ CX, R14
	MOVQ $0, CX
	ADCQ DX, CX

	// | w8 @ R15
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ CX, R15
	MOVQ $0, CX
	ADCQ DX, CX

	// | w9 @ R8
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ CX, R8
	MOVQ $0, CX
	ADCQ DX, CX

	// | w10 @ R9
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ DX, DI
	ADDQ CX, R9
	// | w11 @ R10
	ADCQ DI, R10

	// |  w6,  w7,  w8,  w9, w10, w11
	// | R13, R14, R15,  R8,  R9, R10

/*	 | reduction										*/

	// | c = (w6, w7, w8, w9, w10, w11) @ (R9, R10, R11, DI, CX, R14)
	MOVQ R13, AX
	MOVQ R14, BX
	MOVQ R15, CX
	MOVQ R8, DX
	MOVQ R9, R11
	MOVQ R10, R12
	SUBQ ·modulus+0(SB), AX
	SBBQ ·modulus+8(SB), BX
	SBBQ ·modulus+16(SB), CX
	SBBQ ·modulus+24(SB), DX
	SBBQ ·modulus+32(SB), R11
	SBBQ ·modulus+40(SB), R12
	CMOVQCC AX, R13
	CMOVQCC BX, R14
	CMOVQCC CX, R15
	CMOVQCC DX, R8
	CMOVQCC R11, R9
	CMOVQCC R12, R10

/*	 | out													*/

	MOVQ c+0(FP), SI
	MOVQ R13, (SI)
	MOVQ R14, 8(SI)
	MOVQ R15, 16(SI)
	MOVQ R8, 24(SI)
	MOVQ R9, 32(SI)
	MOVQ R10, 40(SI)

	RET
/*	 | end mont											*/


TEXT ·montmul_nobmi2(SB), NOSPLIT, $16-24

/*	 | inputs							 					*/

	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

/*	 | multiplication phase 				*/

	// | w = a * b
	// | a = (a0, a1, a2, a3, a4, a5)
	// | b = (b0, b1, b2, b3, b4, b5)
	// | w = (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11)

	MOVQ $0, R9
	MOVQ $0, R10
	MOVQ $0, R11
	MOVQ $0, R12
	MOVQ $0, R13
	MOVQ $0, BX

	// | b0 @ CX
	MOVQ (SI), CX

	// | a0 * b0
	// | (w0, w1) @ (SP, R8)
	MOVQ (DI), AX
	MULQ CX
	MOVQ AX, 0(SP)
	MOVQ DX, R8

	// | a1 * b0
	// | (w1, w2) @ (R8, R9)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | a2 * b0
	// | (w2, w3) @ (R9, R10)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | a3 * b0
	// | (w3, w4) @ (R10, R11)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11

	// | a4 * b0
	// | (w4, w5) @ (R11, R12)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12

	// | a5 * b0
	// | (w5, w6) @ (R12, R13)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13

	// | b1 @ CX
	MOVQ 8(SI), CX

	// | a0 * b1
	// | (w1, w2, w3, w4) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w1 @ 8(SP)
	MOVQ R8, 8(SP)

	// | a1 * b1
	// | (w2, w3, w4, w5) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w2 @ R8
	MOVQ R9, R8

	// | a2 * b1
	// | (w3, w4, w5, w6) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w3 @ R9
	MOVQ R10, R9

	// | a3 * b1
	// | (w4, w5, w6, w7) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w4 @ R10
	MOVQ R11, R10

	// | a4 * b1
	// | (w5, w6, w7) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w5 @ R11
	MOVQ R12, R11

	// | a5 * b1
	// | (w6, w7) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w6 @ R12
	MOVQ R13, R12
	// | w7 @ R13
	MOVQ BX, R13

	// | b2 @ CX
	MOVQ 16(SI), CX
	MOVQ $0, BX

	// | a0 * b2
	// | (w2, w3, w4, w5) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w2 @ 8(SP)
	MOVQ R8, 16(SP)

	// | a1 * b2
	// | (w3, w4, w5, w6) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w3 @ R8
	MOVQ R9, R8

	// | a2 * b2
	// | (w4, w5, w6, w7) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w4 @ R9
	MOVQ R10, R9

	// | a3 * b2
	// | (w5, w6, w7, w8) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w5 @ R10
	MOVQ R11, R10

	// | a4 * b2
	// | (w6, w7, w8) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w6 @ R11
	MOVQ R12, R11

	// | a5 * b1
	// | (w7, w8) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w7 @ R12
	MOVQ R13, R12
	// | w8 @ R13
	MOVQ BX, R13

	// | b3 @ CX
	MOVQ 24(SI), CX
	MOVQ $0, BX

	// | a0 * b3
	// | (w3, w4, w5, w6) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w3 @ 8(SP)
	MOVQ R8, R14

	// | a1 * b3
	// | (w4, w5, w6, w7) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w4 @ R8
	MOVQ R9, R8

	// | a2 * b3
	// | (w5, w6, w7, w8) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w5 @ R9
	MOVQ R10, R9

	// | a3 * b3
	// | (w6, w7, w8, w9) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w6 @ R10
	MOVQ R11, R10

	// | a4 * b3
	// | (w7, w8, w9) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w7 @ R11
	MOVQ R12, R11

	// | a5 * b3
	// | (w8, w9) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w8 @ R12
	MOVQ R13, R12
	// | w9 @ R13
	MOVQ BX, R13

	// | b4 @ CX
	MOVQ 32(SI), CX
	MOVQ $0, BX

	// | a0 * b4
	// | (w4, w5, w6, w7) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w4 @ 8(SP)
	MOVQ R8, R15

	// | a1 * b4
	// | (w5, w6, w7, w8) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w5 @ R8
	MOVQ R9, R8

	// | a2 * b4
	// | (w6, w7, w8, w9) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w6 @ R9
	MOVQ R10, R9

	// | a3 * b4
	// | (w7, w8, w9, w10) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w7 @ R10
	MOVQ R11, R10

	// | a4 * b4
	// | (w8, w9, w10) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w8 @ R11
	MOVQ R12, R11

	// | a5 * b4
	// | (w9, w10) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w9 @ R12
	MOVQ R13, R12
	// | w10 @ R13
	MOVQ BX, R13

	// | b5 @ CX
	MOVQ 40(SI), CX
	MOVQ $0, BX

	// | a0 * b5
	// | (w5, w6, w7, w8) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX

	// | a1 * b5
	// | (w6, w7, w8, w9) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX

	// | a2 * b5
	// | (w7, w8, w9, w10) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX

	// | a3 * b5
	// | (w8, w9, w10, w11) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX

	// | a4 * b5
	// | (w9, w10, w11) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX

	// | a5 * b5
	// | (w10, w11) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	 0,   8,  16,  R14, R15,  R8,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// |  R9,  R10, R11, R12, R13,  BX,

	// | 
	// | Montgomerry Reduction Phase
	// | c = w % p

/*	 | swap								 					*/

	MOVQ 0(SP), SI
	MOVQ 8(SP), DI
	MOVQ 16(SP), CX
	MOVQ R13, 0(SP)
	MOVQ BX, 8(SP)
	// | R13 will be the carry register

/*	 | mont_i0											*/

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,   8

	// | i = 0
	// | (u @ BX) = (w0 @ SI) * inverse_p
	MOVQ SI, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// | SI is idle now

	// | w1 @ DI
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, DI
	ADCQ $0, DX
	ADDQ R13, DI
	MOVQ $0, R13
	ADCQ DX, R13

	// | w2 @ CX
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, CX
	ADCQ $0, DX
	ADDQ R13, CX
	MOVQ $0, R13
	ADCQ DX, R13

	// | w3 @ R14
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ R13, R14
	MOVQ $0, R13
	ADCQ DX, R13

	// | w4 @ R15
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	// | w6 @ R9
	ADCQ DX, R9
	
	// | long_carry @ SI should be added to w7
	MOVQ $0, SI
	ADCQ $0, SI

/*	 | mont_i1:						 					*/

	// |  lc,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11,
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,   8,

	// | i = 1
	// | (u @ BX) = (w1 @ DI) * inverse_p
	MOVQ DI, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// | DI is idle now

	// | w2 @ CX
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, CX
	ADCQ $0, DX
	ADDQ R13, CX
	MOVQ $0, R13
	ADCQ DX, R13

	// | w3 @ R14
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ R13, R14
	MOVQ $0, R13
	ADCQ DX, R13

	// | w4 @ R15
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	// | in the last round of the iteration
	// | we don't use the short carry @ R13
	// | instead we bring back long_carry @ SI
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ DX, SI
	ADDQ R13, R9
	// | w7 @ R10
	ADCQ SI, R10
	// | long_carry @ DI should be added to w8
	MOVQ $0, SI
	ADCQ $0, SI
	
/*	 | mont_i2											*/

	// |  lc,  - ,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,   8

	// | i = 2
	// | (u @ BX) = (w2 @ CX) * inverse_p
	MOVQ CX, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// CX is idle now

	// | w3 @ R14
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ R13, R14
	MOVQ $0, R13
	ADCQ DX, R13

	// | w4 @ R15
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

	// | w7 @ R10
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ DX, SI
	ADDQ R13, R10
	// | w8 @ R11
	ADCQ SI, R11
	// | long_carry @ SI should be added to w9
	MOVQ $0, SI
	ADCQ $0, SI

/*	 | mont_i3:						 					*/

	// |  lc,  - ,  - ,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,  8

	// | i = 3
	// | (u @ BX) = (w3 @ R14) * inverse_p
	MOVQ R14, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// R14 is idle now

	// | w4 @ R15
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

	// | w7 @ R10
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ R13, R10
	MOVQ $0, R13
	ADCQ DX, R13

	// | w8 @ R11
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ DX, SI
	ADDQ R13, R11
	// | w9 @ R12
	ADCQ SI, R12
	// | long_carry @ SI should be added to w10
	MOVQ $0, SI
	ADCQ $0, SI
 	
/*	 | mont_i4:						 					*/

	// |  lc,  - ,  - ,  - ,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,  8

	// | i = 4
	// | (u @ BX) = (w4 @ R15) * inverse_p
	MOVQ R15, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// R15 is idle now

	// | w5 @ R8
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

	// | w7 @ R10
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ R13, R10
	MOVQ $0, R13
	ADCQ DX, R13

	// | w8 @ R11
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ R13, R11
	MOVQ $0, R13
	ADCQ DX, R13

	// | w9 @ R12
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ DX, SI
	ADDQ R13, R12

/*	 | swap								 					*/

	// | from stack to available registers
	// | w10 @ CX
	// | w11 @ R14
	MOVQ 0(SP), CX
	MOVQ 8(SP), R14

	// | w10 @ DI
	ADCQ SI, CX
	// | long_carry @ SI should be added to w11
	ADCQ $0, R14

/*	 | mont_i5:							 				*/

	// |  lc,  - ,  - ,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI, R15,  R8,  R9, R10, R11, R12,  CX, R14

	// | i = 5
	// | (u @ BX) = (w5 @ R8) * inverse_p
	MOVQ R8, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// R8 is idle now

	// | w6 @ R9
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

		// | w7 @ R10
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ R13, R10
	MOVQ $0, R13
	ADCQ DX, R13

	// | w8 @ R11
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ R13, R11
	MOVQ $0, R13
	ADCQ DX, R13

	// | w9 @ R12
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ $0, DX
	ADDQ R13, R12
	ADCQ DX, CX
	ADCQ $0, R14

	// | (w10, w11) @ (CX, R14)
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, CX
	ADCQ DX, R14

/*	 | reduction										*/

	// | c = (w6, w7, w8, w9, w10, w11) @ (R9, R10, R11, DI, CX, R14)
	MOVQ R9, AX
	MOVQ R10, BX
	MOVQ R11, DX
	MOVQ R12, R8
	MOVQ CX, R15
	MOVQ R14, R13
	SUBQ ·modulus+0(SB), AX
	SBBQ ·modulus+8(SB), BX
	SBBQ ·modulus+16(SB), DX
	SBBQ ·modulus+24(SB), R8
	SBBQ ·modulus+32(SB), R15
	SBBQ ·modulus+40(SB), R13
	CMOVQCC AX, R9
	CMOVQCC BX, R10
	CMOVQCC DX, R11
	CMOVQCC R8, R12
	CMOVQCC R15, CX
	CMOVQCC R13, R14

/*	 | out													*/

	MOVQ c+0(FP), SI
	MOVQ R9, (SI)
	MOVQ R10, 8(SI)
	MOVQ R11, 16(SI)
	MOVQ R12, 24(SI)
	MOVQ CX, 32(SI)
	MOVQ R14, 40(SI)
	RET

/*	 | end													*/


TEXT ·montmul_assign_nobmi2(SB), NOSPLIT, $16-16

/*	 | inputs							 					*/

	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

/*	 | multiplication phase 				*/

	// | w = a * b
	// | a = (a0, a1, a2, a3, a4, a5)
	// | b = (b0, b1, b2, b3, b4, b5)
	// | w = (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11)

	MOVQ $0, R9
	MOVQ $0, R10
	MOVQ $0, R11
	MOVQ $0, R12
	MOVQ $0, R13
	MOVQ $0, BX

	// | b0 @ CX
	MOVQ (SI), CX

	// | a0 * b0
	// | (w0, w1) @ (SP, R8)
	MOVQ (DI), AX
	MULQ CX
	MOVQ AX, 0(SP)
	MOVQ DX, R8

	// | a1 * b0
	// | (w1, w2) @ (R8, R9)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | a2 * b0
	// | (w2, w3) @ (R9, R10)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | a3 * b0
	// | (w3, w4) @ (R10, R11)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11

	// | a4 * b0
	// | (w4, w5) @ (R11, R12)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12

	// | a5 * b0
	// | (w5, w6) @ (R12, R13)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13

	// | b1 @ CX
	MOVQ 8(SI), CX

	// | a0 * b1
	// | (w1, w2, w3, w4) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w1 @ 8(SP)
	MOVQ R8, 8(SP)

	// | a1 * b1
	// | (w2, w3, w4, w5) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w2 @ R8
	MOVQ R9, R8

	// | a2 * b1
	// | (w3, w4, w5, w6) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w3 @ R9
	MOVQ R10, R9

	// | a3 * b1
	// | (w4, w5, w6, w7) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w4 @ R10
	MOVQ R11, R10

	// | a4 * b1
	// | (w5, w6, w7) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w5 @ R11
	MOVQ R12, R11

	// | a5 * b1
	// | (w6, w7) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w6 @ R12
	MOVQ R13, R12
	// | w7 @ R13
	MOVQ BX, R13

	// | b2 @ CX
	MOVQ 16(SI), CX
	MOVQ $0, BX

	// | a0 * b2
	// | (w2, w3, w4, w5) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w2 @ 8(SP)
	MOVQ R8, 16(SP)

	// | a1 * b2
	// | (w3, w4, w5, w6) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w3 @ R8
	MOVQ R9, R8

	// | a2 * b2
	// | (w4, w5, w6, w7) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w4 @ R9
	MOVQ R10, R9

	// | a3 * b2
	// | (w5, w6, w7, w8) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w5 @ R10
	MOVQ R11, R10

	// | a4 * b2
	// | (w6, w7, w8) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w6 @ R11
	MOVQ R12, R11

	// | a5 * b1
	// | (w7, w8) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w7 @ R12
	MOVQ R13, R12
	// | w8 @ R13
	MOVQ BX, R13

	// | b3 @ CX
	MOVQ 24(SI), CX
	MOVQ $0, BX

	// | a0 * b3
	// | (w3, w4, w5, w6) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w3 @ 8(SP)
	MOVQ R8, R14

	// | a1 * b3
	// | (w4, w5, w6, w7) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w4 @ R8
	MOVQ R9, R8

	// | a2 * b3
	// | (w5, w6, w7, w8) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w5 @ R9
	MOVQ R10, R9

	// | a3 * b3
	// | (w6, w7, w8, w9) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w6 @ R10
	MOVQ R11, R10

	// | a4 * b3
	// | (w7, w8, w9) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w7 @ R11
	MOVQ R12, R11

	// | a5 * b3
	// | (w8, w9) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w8 @ R12
	MOVQ R13, R12
	// | w9 @ R13
	MOVQ BX, R13

	// | b4 @ CX
	MOVQ 32(SI), CX
	MOVQ $0, BX

	// | a0 * b4
	// | (w4, w5, w6, w7) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX
	// | w4 @ 8(SP)
	MOVQ R8, R15

	// | a1 * b4
	// | (w5, w6, w7, w8) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX
	// | w5 @ R8
	MOVQ R9, R8

	// | a2 * b4
	// | (w6, w7, w8, w9) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX
	// | w6 @ R9
	MOVQ R10, R9

	// | a3 * b4
	// | (w7, w8, w9, w10) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX
	// | w7 @ R10
	MOVQ R11, R10

	// | a4 * b4
	// | (w8, w9, w10) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX
	// | w8 @ R11
	MOVQ R12, R11

	// | a5 * b4
	// | (w9, w10) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX
	// | w9 @ R12
	MOVQ R13, R12
	// | w10 @ R13
	MOVQ BX, R13

	// | b5 @ CX
	MOVQ 40(SI), CX
	MOVQ $0, BX

	// | a0 * b5
	// | (w5, w6, w7, w8) @ (R8, R9, R10, BX)
	MOVQ (DI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0, R10
	ADCQ $0, BX

	// | a1 * b5
	// | (w6, w7, w8, w9) @ (R9, R10, R11, BX)
	MOVQ 8(DI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0, BX
	ADCQ $0, BX

	// | a2 * b5
	// | (w7, w8, w9, w10) @ (R10, R11, R12, BX)
	MOVQ 16(DI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0, BX
	ADCQ $0, BX

	// | a3 * b5
	// | (w8, w9, w10, w11) @ (R11, R12, R13, BX)
	MOVQ 24(DI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0, BX
	ADCQ $0, BX

	// | a4 * b5
	// | (w9, w10, w11) @ (R12, R13, BX)
	MOVQ 32(DI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0, BX

	// | a5 * b5
	// | (w10, w11) @ (R13, BX)
	MOVQ 40(DI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX

	// |	w0,  w1,  w2,   w3,  w4,  w5,
	// | 	 0,   8,  16,  R14, R15,  R8,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// |  R9,  R10, R11, R12, R13,  BX,

	// | 
	// | Montgomerry Reduction Phase
	// | c = w % p

/*	 | swap								 					*/

	MOVQ 0(SP), SI
	MOVQ 8(SP), DI
	MOVQ 16(SP), CX
	MOVQ R13, 0(SP)
	MOVQ BX, 8(SP)
	// | R13 will be the carry register

/*	 | mont_i0											*/

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,   8

	// | i = 0
	// | (u @ BX) = (w0 @ SI) * inverse_p
	MOVQ SI, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// | SI is idle now

	// | w1 @ DI
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, DI
	ADCQ $0, DX
	ADDQ R13, DI
	MOVQ $0, R13
	ADCQ DX, R13

	// | w2 @ CX
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, CX
	ADCQ $0, DX
	ADDQ R13, CX
	MOVQ $0, R13
	ADCQ DX, R13

	// | w3 @ R14
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ R13, R14
	MOVQ $0, R13
	ADCQ DX, R13

	// | w4 @ R15
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	// | w6 @ R9
	ADCQ DX, R9
	
	// | long_carry @ SI should be added to w7
	MOVQ $0, SI
	ADCQ $0, SI

/*	 | mont_i1:						 					*/

	// |  lc,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11,
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,   8,

	// | i = 1
	// | (u @ BX) = (w1 @ DI) * inverse_p
	MOVQ DI, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// | DI is idle now

	// | w2 @ CX
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, CX
	ADCQ $0, DX
	ADDQ R13, CX
	MOVQ $0, R13
	ADCQ DX, R13

	// | w3 @ R14
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ R13, R14
	MOVQ $0, R13
	ADCQ DX, R13

	// | w4 @ R15
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	// | in the last round of the iteration
	// | we don't use the short carry @ R13
	// | instead we bring back long_carry @ SI
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ DX, SI
	ADDQ R13, R9
	// | w7 @ R10
	ADCQ SI, R10
	// | long_carry @ DI should be added to w8
	MOVQ $0, SI
	ADCQ $0, SI
	
/*	 | mont_i2											*/

	// |  lc,  - ,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,   8

	// | i = 2
	// | (u @ BX) = (w2 @ CX) * inverse_p
	MOVQ CX, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// CX is idle now

	// | w3 @ R14
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R14
	ADCQ $0, DX
	ADDQ R13, R14
	MOVQ $0, R13
	ADCQ DX, R13

	// | w4 @ R15
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

	// | w7 @ R10
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ DX, SI
	ADDQ R13, R10
	// | w8 @ R11
	ADCQ SI, R11
	// | long_carry @ SI should be added to w9
	MOVQ $0, SI
	ADCQ $0, SI

/*	 | mont_i3:						 					*/

	// |  lc,  - ,  - ,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,  8

	// | i = 3
	// | (u @ BX) = (w3 @ R14) * inverse_p
	MOVQ R14, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// R14 is idle now

	// | w4 @ R15
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R15
	ADCQ $0, DX
	ADDQ R13, R15
	MOVQ $0, R13
	ADCQ DX, R13

	// | w5 @ R8
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

	// | w7 @ R10
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ R13, R10
	MOVQ $0, R13
	ADCQ DX, R13

	// | w8 @ R11
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ DX, SI
	ADDQ R13, R11
	// | w9 @ R12
	ADCQ SI, R12
	// | long_carry @ SI should be added to w10
	MOVQ $0, SI
	ADCQ $0, SI
 	
/*	 | mont_i4:						 					*/

	// |  lc,  - ,  - ,  - ,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI,  CX, R14, R15,  R8,  R9, R10, R11, R12,   0,  8

	// | i = 4
	// | (u @ BX) = (w4 @ R15) * inverse_p
	MOVQ R15, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// R15 is idle now

	// | w5 @ R8
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R8
	ADCQ $0, DX
	ADDQ R13, R8
	MOVQ $0, R13
	ADCQ DX, R13

	// | w6 @ R9
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

	// | w7 @ R10
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ R13, R10
	MOVQ $0, R13
	ADCQ DX, R13

	// | w8 @ R11
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ R13, R11
	MOVQ $0, R13
	ADCQ DX, R13

	// | w9 @ R12
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ DX, SI
	ADDQ R13, R12

/*	 | swap								 					*/

	// | from stack to available registers
	// | w10 @ CX
	// | w11 @ R14
	MOVQ 0(SP), CX
	MOVQ 8(SP), R14

	// | w10 @ DI
	ADCQ SI, CX
	// | long_carry @ SI should be added to w11
	ADCQ $0, R14

/*	 | mont_i5:						 					*/

	// |  lc,  - ,  - ,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  SI,  DI, R15,  R8,  R9, R10, R11, R12,  CX, R14

	// | i = 5
	// | (u @ BX) = (w5 @ R8) * inverse_p
	MOVQ R8, AX
	MULQ ·inp+0(SB)
	MOVQ AX, BX
	MOVQ $0, R13
	MOVQ ·modulus+0(SB), AX
	MULQ BX
	ADCQ DX, R13
	// R8 is idle now

	// | w6 @ R9
	MOVQ ·modulus+8(SB), AX
	MULQ BX
	ADDQ AX, R9
	ADCQ $0, DX
	ADDQ R13, R9
	MOVQ $0, R13
	ADCQ DX, R13

		// | w7 @ R10
	MOVQ ·modulus+16(SB), AX
	MULQ BX
	ADDQ AX, R10
	ADCQ $0, DX
	ADDQ R13, R10
	MOVQ $0, R13
	ADCQ DX, R13

	// | w8 @ R11
	MOVQ ·modulus+24(SB), AX
	MULQ BX
	ADDQ AX, R11
	ADCQ $0, DX
	ADDQ R13, R11
	MOVQ $0, R13
	ADCQ DX, R13

	// | w9 @ R12
	MOVQ ·modulus+32(SB), AX
	MULQ BX
	ADDQ AX, R12
	ADCQ $0, DX
	ADDQ R13, R12
	ADCQ DX, CX
	ADCQ $0, R14

	// | (w10, w11) @ (CX, R14)
	MOVQ ·modulus+40(SB), AX
	MULQ BX
	ADDQ AX, CX
	ADCQ DX, R14

/*	 | reduction										*/

	// | c = (w6, w7, w8, w9, w10, w11) @ (R9, R10, R11, DI, CX, R14)
	MOVQ R9, AX
	MOVQ R10, BX
	MOVQ R11, DX
	MOVQ R12, R8
	MOVQ CX, R15
	MOVQ R14, R13
	SUBQ ·modulus+0(SB), AX
	SBBQ ·modulus+8(SB), BX
	SBBQ ·modulus+16(SB), DX
	SBBQ ·modulus+24(SB), R8
	SBBQ ·modulus+32(SB), R15
	SBBQ ·modulus+40(SB), R13
	CMOVQCC AX, R9
	CMOVQCC BX, R10
	CMOVQCC DX, R11
	CMOVQCC R8, R12
	CMOVQCC R15, CX
	CMOVQCC R13, R14

/*	 | out													*/

	MOVQ a+0(FP), SI
	MOVQ R9, (SI)
	MOVQ R10, 8(SI)
	MOVQ R11, 16(SI)
	MOVQ R12, 24(SI)
	MOVQ CX, 32(SI)
	MOVQ R14, 40(SI)
	RET

/*	 | end													*/

