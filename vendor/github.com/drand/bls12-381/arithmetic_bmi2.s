#include "textflag.h"


TEXT ·montmul_bmi2(SB), NOSPLIT, $16-24

/*	 | inputs							 	*/

	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

/*	 | multiplication phase 	*/

	// | w = a * b
	// | a = (a0, a1, a2, a3, a4, a5)
	// | b = (b0, b1, b2, b3, b4, b5)
	// | w = (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11)

/*	 | i0									 	*/

	MOVQ (SI), DX
	XORQ AX, AX

	MULXQ (DI), AX, R8
	MOVQ AX, CX

	MULXQ 8(DI), AX, R9
	ADCXQ AX, R8

	MULXQ 16(DI), AX, R10
	ADCXQ AX, R9

	MULXQ 24(DI), AX, R11
	ADCXQ AX, R10

	MULXQ 32(DI), AX, R12
	ADCXQ AX, R11

	MULXQ 40(DI), AX, R13
	ADCXQ AX, R12
	ADCQ $0, R13

/*	 | i1									 	*/

	MOVQ 8(SI), DX
	XORQ R14, R14

	MULXQ (DI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	MOVQ R8, (SP)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R13
	ADOXQ R14, R14
	ADCXQ BX, R14

/*	 | i2									 	*/

	MOVQ 16(SI), DX
	XORQ R15, R15

	MULXQ (DI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	MOVQ R9, 8(SP)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R14
	ADOXQ R15, R15
	ADCXQ BX, R15

/*	 | i3									 	*/

	MOVQ 24(SI), DX
	XORQ R8, R8

	MULXQ (DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R15
	ADOXQ R8, R8
	ADCXQ BX, R8

/*	 | i4									 	*/

	MOVQ 32(SI), DX
	XORQ R9, R9

	MULXQ (DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R8
	ADOXQ R9, R9
	ADCXQ BX, R9

/*	 | i5									 	*/

	MOVQ 40(SI), DX
	XORQ SI, SI

	MULXQ (DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R9
	ADOXQ BX, SI

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,   0,   8,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9,  SI,

	// | 
/*	 | montgomerry reduction	*/
	// | c = w % p

	MOVQ (SP), DI
	MOVQ 8(SP), BX
	MOVQ R9, (SP)
	MOVQ SI, 8(SP)

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

/*	 | i0											*/

	MOVQ ·inp+0(SB), DX
	MULXQ CX, DX, R9

	XORQ SI, SI
	MULXQ ·modulus+0(SB), AX, R9
	ADOXQ AX, CX
	ADCXQ R9, DI

	MULXQ ·modulus+8(SB), AX, R9
	ADOXQ AX, DI
	ADCXQ R9, BX

	MULXQ ·modulus+16(SB), AX, R9
	ADOXQ AX, BX
	ADCXQ R9, R10

	MULXQ ·modulus+24(SB), AX, R9
	ADOXQ AX, R10
	ADCXQ R9, R11

	MULXQ ·modulus+32(SB), AX, R9
	ADOXQ AX, R11
	ADCXQ R9, R12

	MULXQ ·modulus+40(SB), AX, R9
	ADOXQ AX, R12
	ADCXQ R9, R13
	ADOXQ SI, R13
	ADCXQ SI, SI

/*	 | i1											*/

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// |  CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

	XORQ CX, CX
	MOVQ DI, DX
	MULXQ ·inp+0(SB), DX, R9

	MULXQ ·modulus+0(SB), AX, R9
	ADOXQ AX, DI
	ADCXQ R9, BX

	MULXQ ·modulus+8(SB), AX, R9
	ADOXQ AX, BX
	ADCXQ R9, R10

	MULXQ ·modulus+16(SB), AX, R9
	ADOXQ AX, R10
	ADCXQ R9, R11

	MULXQ ·modulus+24(SB), AX, R9
	ADOXQ AX, R11
	ADCXQ R9, R12

	MULXQ ·modulus+32(SB), AX, R9
	ADOXQ AX, R12
	ADCXQ R9, R13

	MULXQ ·modulus+40(SB), AX, R9
	ADOXQ AX, R13
	ADCXQ R9, R14
	ADOXQ SI, R14
	ADCXQ CX, CX

/*	 | i2											*/

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

	XORQ DI, DI
	MOVQ BX, DX
	MULXQ ·inp+0(SB), DX, R9

	MULXQ ·modulus+0(SB), AX, R9
	ADOXQ AX, BX
	ADCXQ R9, R10

	MULXQ ·modulus+8(SB), AX, R9
	ADOXQ AX, R10
	ADCXQ R9, R11

	MULXQ ·modulus+16(SB), AX, R9
	ADOXQ AX, R11
	ADCXQ R9, R12

	MULXQ ·modulus+24(SB), AX, R9
	ADOXQ AX, R12
	ADCXQ R9, R13

	MULXQ ·modulus+32(SB), AX, R9
	ADOXQ AX, R13
	ADCXQ R9, R14

	MULXQ ·modulus+40(SB), AX, R9
	ADOXQ AX, R14
	ADCXQ R9, R15
	ADOXQ CX, R15
	ADCXQ DI, DI

/*	 | i3											*/

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

	XORQ CX, CX
	MOVQ R10, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8
	ADOXQ DI, R8
	ADCXQ CX, CX

/*	 | i4											*/

	MOVQ (SP), R9

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	 -,   -,   -,   -,  R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9,   8,

	XORQ DI, DI
	MOVQ R11, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	ADOXQ CX, R9
	ADCXQ DI, DI

/*	 | i5											*/

	MOVQ 8(SP), R10

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	 -,   -,   -,   -,    -, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9, R10,

	XORQ AX, AX
	MOVQ R12, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	ADOXQ DI, R10

/*	 | reduction					 		*/

	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9, R10,

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

/*	 | out								 		*/

	MOVQ c+0(FP), SI
	MOVQ R13, (SI)
	MOVQ R14, 8(SI)
	MOVQ R15, 16(SI)
	MOVQ R8, 24(SI)
	MOVQ R9, 32(SI)
	MOVQ R10, 40(SI)
	RET

/*	 | end										*/


TEXT ·mont_bmi2(SB), NOSPLIT, $0-16

/*	 | inputs									*/

	MOVQ a+8(FP), SI
	MOVQ 8(SI), R8
	MOVQ 16(SI), R9
	MOVQ 24(SI), R10
	MOVQ 32(SI), R11
	MOVQ 40(SI), R12
	MOVQ 48(SI), R13
	MOVQ 56(SI), R14
	MOVQ 64(SI), R15

/*	 | i0											*/

	MOVQ 0(SI), CX
	MOVQ ·inp+0(SB), DX
	MULXQ CX, DX, BX

	XORQ DI, DI
	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12
	
	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13
	ADOXQ DI, R13
	ADCXQ DI, DI

	// |  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R8,  R9, R10, R11, R12, R13, R14,   -,   -,   -,   -

/*	 | i1											*/

	XORQ CX, CX
	MOVQ R8, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14
	ADOXQ DI, R14
	ADCXQ CX, CX

	// |   -,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R8,  R9, R10, R11, R12, R13, R14,   -,   -,   -,   -

/*   | i2											*/

	XORQ DI, DI
	MOVQ R9, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15
	ADOXQ CX, R15
	ADCXQ DI, DI

	// |   -,   -,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R8,  R9, R10, R11, R12, R13, R14, R15,   -,   -,   -

/*	 | i3											*/

	MOVQ 72(SI), R8
	XORQ CX, CX
	MOVQ R10, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8
	ADOXQ DI, R8
	ADCXQ CX, CX

	// |   -,   -,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// |  R9, R10, R11, R12, R13, R14, R15,  R8,   -,   -

/*	 | i4											*/

	MOVQ 80(SI), R9
	XORQ DI, DI
	MOVQ R11, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	ADOXQ CX, R9
	ADCXQ DI, DI 

	// |   -,   -,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | R10, R11, R12, R13, R14, R15,  R8,  R9,   -

/*	 | i5											*/

	MOVQ 88(SI), R10
	XORQ AX, AX
	MOVQ R12, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	ADOXQ DI, R10

	// |  w6,  w7,  w8,  w9, w10, w11
	// | R13, R14, R15,  R8,  R9, R10

/*	 | reduction					 		*/

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

/*	 | out								 		*/

	MOVQ c+0(FP), SI
	MOVQ R13, (SI)
	MOVQ R14, 8(SI)
	MOVQ R15, 16(SI)
	MOVQ R8, 24(SI)
	MOVQ R9, 32(SI)
	MOVQ R10, 40(SI)
	RET

/*	 | end								 		*/


TEXT ·mul_bmi2(SB), NOSPLIT, $0-24

/*	 | inputs							 	*/

	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI
	MOVQ c+0(FP), CX

	// | w = a * b
	// | a = (a0, a1, a2, a3, a4, a5)
	// | b = (b0, b1, b2, b3, b4, b5)
	// | w = (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11)

/*	 | i0									 	*/

	// | b0 @ CX
	MOVQ (SI), DX
	XORQ AX, AX

	MULXQ (DI), AX, R8
	MOVQ AX, (CX)

	MULXQ 8(DI), AX, R9
	ADCXQ AX, R8

	MULXQ 16(DI), AX, R10
	ADCXQ AX, R9

	MULXQ 24(DI), AX, R11
	ADCXQ AX, R10

	MULXQ 32(DI), AX, R12
	ADCXQ AX, R11
	//
	MULXQ 40(DI), AX, R13
	ADCXQ AX, R12
	ADCQ $0, R13

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | out,  R8,  R9, R10, R11, R12, R13,   -,  -,   -,   -,   -

/*	 | i1									 	*/

	MOVQ 8(SI), DX
	XORQ R14, R14

	MULXQ (DI), AX, BX
	ADCXQ AX, R8
	ADOXQ BX, R9
	MOVQ R8, 8(CX)

	MULXQ 8(DI), AX, BX
	ADCXQ AX, R9
	ADOXQ BX, R10

	MULXQ 16(DI), AX, BX
	ADCXQ AX, R10
	ADOXQ BX, R11

	MULXQ 24(DI), AX, BX
	ADCXQ AX, R11
	ADOXQ BX, R12

	MULXQ 32(DI), AX, BX
	ADCXQ AX, R12
	ADOXQ BX, R13

	MULXQ 40(DI), AX, BX
	ADCXQ AX, R13
	ADCXQ R14, R14
	ADOXQ BX, R14

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | out, out,  R9, R10, R11, R12, R13, R14,  -,   -,   -,   -

/*	 | i2									 	*/

	MOVQ 16(SI), DX
	XORQ R15, R15

	MULXQ (DI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	MOVQ R9, 16(CX)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R14
	ADOXQ R15, R15
	ADCXQ BX, R15

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | out, out, out, R10, R11, R12, R13, R14,  R15,   -,   -,   -

/*	 | i3									 	*/

	MOVQ 24(SI), DX
	XORQ R8, R8

	MULXQ (DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11
	MOVQ R10, 24(CX)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R15
	ADOXQ R8, R8
	ADCXQ BX, R8

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | out, out, out, out, R11, R12, R13, R14, R15,  R8,   -,   -

/*	 | i4									 	*/

	MOVQ 32(SI), DX
	XORQ R9, R9

	MULXQ (DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12
	MOVQ R11, 32(CX)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R8
	ADOXQ R9, R9
	ADCXQ BX, R9

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | out, out, out, out, out, R12, R13, R14, R15,  R8,  R9,   -

/*	 | i5									 	*/

	MOVQ 40(SI), DX
	XORQ R10, R10

	MULXQ (DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13
	MOVQ R12, 40(CX)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R9
	ADOXQ BX, R10

	// |  w0,  w1,  w2,  w3,  w4,  w5,  w6,  w7,  w8,  w9, w10, w11
	// | out, out, out, out, out, out, R13, R14, R15,  R8,  R9,   -

	MOVQ R13, 48(CX)
	MOVQ R14, 56(CX)
	MOVQ R15, 64(CX)
	MOVQ R8, 72(CX)
	MOVQ R9, 80(CX)
	MOVQ R10, 88(CX)

	RET

/*	 | end										*/	


TEXT ·montmul_assign_bmi2(SB), NOSPLIT, $16-16

/*	 | inputs							 	*/

	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

/*	 | multiplication phase 	*/

	// | w = a * b
	// | a = (a0, a1, a2, a3, a4, a5)
	// | b = (b0, b1, b2, b3, b4, b5)
	// | w = (w0, w1, w2, w3, w4, w5, w6, w7, w8, w9, w10, w11)

	/*	 | i0									 */

	MOVQ (SI), DX
	XORQ AX, AX

	MULXQ (DI), AX, R8
	MOVQ AX, CX

	MULXQ 8(DI), AX, R9
	ADCXQ AX, R8

	MULXQ 16(DI), AX, R10
	ADCXQ AX, R9

	MULXQ 24(DI), AX, R11
	ADCXQ AX, R10

	MULXQ 32(DI), AX, R12
	ADCXQ AX, R11

	MULXQ 40(DI), AX, R13
	ADCXQ AX, R12
	ADCQ $0, R13

/*	 | i1									 	*/

	MOVQ 8(SI), DX
	XORQ R14, R14

	MULXQ (DI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	MOVQ R8, (SP)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R13
	ADOXQ R14, R14
	ADCXQ BX, R14

/*	 | i2									 	*/

	MOVQ 16(SI), DX
	XORQ R15, R15

	MULXQ (DI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	MOVQ R9, 8(SP)

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R14
	ADOXQ R15, R15
	ADCXQ BX, R15

/*	 | i3									 	*/

	MOVQ 24(SI), DX
	XORQ R8, R8

	MULXQ (DI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R15
	ADOXQ R8, R8
	ADCXQ BX, R8

/*	 | i4									 	*/

	MOVQ 32(SI), DX
	XORQ R9, R9
	
	MULXQ (DI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R8
	ADOXQ R9, R9
	ADCXQ BX, R9

/*	 | i5									 	*/

	MOVQ 40(SI), DX
	XORQ SI, SI

	MULXQ (DI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ 8(DI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ 16(DI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ 24(DI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ 32(DI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ 40(DI), AX, BX
	ADOXQ AX, R9
	ADOXQ BX, SI

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,   0,   8,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9,  SI,

	// | 
/*	 | montgomerry reduction	*/
	// | c = w % p

	MOVQ (SP), DI
	MOVQ 8(SP), BX
	MOVQ R9, (SP)
	MOVQ SI, 8(SP)

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

/*	 | i0											*/

	MOVQ ·inp+0(SB), DX
	MULXQ CX, DX, R9

	XORQ SI, SI
	MULXQ ·modulus+0(SB), AX, R9
	ADOXQ AX, CX
	ADCXQ R9, DI

	MULXQ ·modulus+8(SB), AX, R9
	ADOXQ AX, DI
	ADCXQ R9, BX

	MULXQ ·modulus+16(SB), AX, R9
	ADOXQ AX, BX
	ADCXQ R9, R10

	MULXQ ·modulus+24(SB), AX, R9
	ADOXQ AX, R10
	ADCXQ R9, R11

	MULXQ ·modulus+32(SB), AX, R9
	ADOXQ AX, R11
	ADCXQ R9, R12

	MULXQ ·modulus+40(SB), AX, R9
	ADOXQ AX, R12
	ADCXQ R9, R13
	ADOXQ SI, R13
	ADCXQ SI, SI

/*	 | i1											*/

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

	XORQ CX, CX
	MOVQ DI, DX
	MULXQ ·inp+0(SB), DX, R9

	MULXQ ·modulus+0(SB), AX, R9
	ADOXQ AX, DI
	ADCXQ R9, BX

	MULXQ ·modulus+8(SB), AX, R9
	ADOXQ AX, BX
	ADCXQ R9, R10

	MULXQ ·modulus+16(SB), AX, R9
	ADOXQ AX, R10
	ADCXQ R9, R11

	MULXQ ·modulus+24(SB), AX, R9
	ADOXQ AX, R11
	ADCXQ R9, R12

	MULXQ ·modulus+32(SB), AX, R9
	ADOXQ AX, R12
	ADCXQ R9, R13

	MULXQ ·modulus+40(SB), AX, R9
	ADOXQ AX, R13
	ADCXQ R9, R14
	ADOXQ SI, R14
	ADCXQ CX, CX

/*	 | i2											*/

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

	XORQ DI, DI
	MOVQ BX, DX
	MULXQ ·inp+0(SB), DX, R9

	MULXQ ·modulus+0(SB), AX, R9
	ADOXQ AX, BX
	ADCXQ R9, R10

	MULXQ ·modulus+8(SB), AX, R9
	ADOXQ AX, R10
	ADCXQ R9, R11

	MULXQ ·modulus+16(SB), AX, R9
	ADOXQ AX, R11
	ADCXQ R9, R12

	MULXQ ·modulus+24(SB), AX, R9
	ADOXQ AX, R12
	ADCXQ R9, R13

	MULXQ ·modulus+32(SB), AX, R9
	ADOXQ AX, R13
	ADCXQ R9, R14

	MULXQ ·modulus+40(SB), AX, R9
	ADOXQ AX, R14
	ADCXQ R9, R15
	ADOXQ CX, R15
	ADCXQ DI, DI

/*	 | i3											*/

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	CX,  DI,  BX,  R10, R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,    0,   8,

	XORQ CX, CX
	MOVQ R10, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8
	ADOXQ DI, R8
	ADCXQ CX, CX

/*	 | i4											*/

	MOVQ (SP), R9

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	 -,   -,   -,   -,  R11, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9,   8,

	XORQ DI, DI
	MOVQ R11, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	ADOXQ CX, R9
	ADCXQ DI, DI

/*	 | i5											*/

	MOVQ 8(SP), R10

	// |  w0,  w1,  w2,   w3,  w4,  w5,
	// | 	 -,   -,   -,   -,    -, R12,
	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9, R10,

	XORQ AX, AX
	MOVQ R12, DX
	MULXQ ·inp+0(SB), DX, BX

	MULXQ ·modulus+0(SB), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	MULXQ ·modulus+8(SB), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	MULXQ ·modulus+16(SB), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	MULXQ ·modulus+24(SB), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, R8

	MULXQ ·modulus+32(SB), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	MULXQ ·modulus+40(SB), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	ADOXQ DI, R10

/*	 | reduction					 		*/

	// |  w6,  w7,  w8,  w9,  w10, w11,
	// | R13, R14, R15,  R8,   R9, R10,

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

/*	 | out								 		*/

	MOVQ a+0(FP), SI
	MOVQ R13, (SI)
	MOVQ R14, 8(SI)
	MOVQ R15, 16(SI)
	MOVQ R8, 24(SI)
	MOVQ R9, 32(SI)
	MOVQ R10, 40(SI)
	RET

/*	 | end								 		*/
