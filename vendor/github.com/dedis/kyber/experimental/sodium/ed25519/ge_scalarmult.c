#include "ge.h"
#include "crypto_uint32.h"

#ifdef __cplusplus
# if __GNUC__
#  pragma GCC diagnostic ignored "-Wlong-long"
# endif
#endif

static unsigned char equal(signed char b,signed char c)
{
  unsigned char ub = b;
  unsigned char uc = c;
  unsigned char x = ub ^ uc; /* 0: yes; 1..255: no */
  crypto_uint32 y = x; /* 0: yes; 1..255: no */
  y -= 1; /* 4294967295: yes; 0..254: no */
  y >>= 31; /* 1: yes; 0: no */
  return y;
}

static unsigned char negative(signed char b)
{
  unsigned long long x = b; /* 18446744073709551361..18446744073709551615: yes; 0..255: no */
  x >>= 63; /* 1: yes; 0: no */
  return x;
}

static void cmov(ge_cached *t,ge_cached *u,unsigned char b)
{
  fe_cmov(t->YplusX,u->YplusX,b);
  fe_cmov(t->YminusX,u->YminusX,b);
  fe_cmov(t->Z,u->Z,b);
  fe_cmov(t->T2d,u->T2d,b);
}

static void ge_select(ge_cached *t, ge_cached Ai[8], signed char b)
{
  ge_cached minust;
  unsigned char bnegative = negative(b);
  unsigned char babs = b - (((-bnegative) & b) << 1);

  // conditionally pick cached multiplier for exponent value 0 through 8
  ge_cached_0(t);
  cmov(t,&Ai[0],equal(babs,1));
  cmov(t,&Ai[1],equal(babs,2));
  cmov(t,&Ai[2],equal(babs,3));
  cmov(t,&Ai[3],equal(babs,4));
  cmov(t,&Ai[4],equal(babs,5));
  cmov(t,&Ai[5],equal(babs,6));
  cmov(t,&Ai[6],equal(babs,7));
  cmov(t,&Ai[7],equal(babs,8));

  // compute negated version, conditionally use it
  fe_copy(minust.YplusX,t->YminusX);
  fe_copy(minust.YminusX,t->YplusX);
  fe_copy(minust.Z,t->Z);
  fe_neg(minust.T2d,t->T2d);
  cmov(t,&minust,bnegative);
}

/*
h = a * A
where a = a[0]+256*a[1]+...+256^31 a[31]

Preconditions:
  a[31] <= 127
*/
void ge_scalarmult(ge_p3 *h, const unsigned char *a, const ge_p3 *A)
{
  signed char e[64];
  signed char carry;
  ge_cached Ai[8]; /* A,2A,3A,4A,5A,6A,7A,8A */
  ge_p1p1 r;
  ge_p3 u;
  ge_p2 s;
  ge_cached t;
  int i;

  for (i = 0;i < 32;++i) {
    e[2 * i + 0] = (a[i] >> 0) & 15;
    e[2 * i + 1] = (a[i] >> 4) & 15;
  }
  /* each e[i] is between 0 and 15 */
  /* e[63] is between 0 and 7 */

  carry = 0;
  for (i = 0;i < 63;++i) {
    e[i] += carry;
    carry = e[i] + 8;
    carry >>= 4;
    e[i] -= carry << 4;
  }
  e[63] += carry;
  /* each e[i] is between -8 and 8 */

  /* compute cached array of multiples of A from 1A through 8A */
  ge_p3_to_cached(&Ai[0],A);
  ge_add(&r,A,&Ai[0]); ge_p1p1_to_p3(&u,&r); ge_p3_to_cached(&Ai[1],&u);
  ge_add(&r,A,&Ai[1]); ge_p1p1_to_p3(&u,&r); ge_p3_to_cached(&Ai[2],&u);
  ge_add(&r,A,&Ai[2]); ge_p1p1_to_p3(&u,&r); ge_p3_to_cached(&Ai[3],&u);
  ge_add(&r,A,&Ai[3]); ge_p1p1_to_p3(&u,&r); ge_p3_to_cached(&Ai[4],&u);
  ge_add(&r,A,&Ai[4]); ge_p1p1_to_p3(&u,&r); ge_p3_to_cached(&Ai[5],&u);
  ge_add(&r,A,&Ai[5]); ge_p1p1_to_p3(&u,&r); ge_p3_to_cached(&Ai[6],&u);
  ge_add(&r,A,&Ai[6]); ge_p1p1_to_p3(&u,&r); ge_p3_to_cached(&Ai[7],&u);

  /* special case for first iteration i == 63 */
  ge_p3_0(&u);
  ge_select(&t, Ai, e[63]);
  ge_add(&r, &u, &t);

  for (i = 62; i >= 0; --i) {

    // r <<= 4
    ge_p1p1_to_p2(&s,&r); ge_p2_dbl(&r,&s);
    ge_p1p1_to_p2(&s,&r); ge_p2_dbl(&r,&s);
    ge_p1p1_to_p2(&s,&r); ge_p2_dbl(&r,&s);
    ge_p1p1_to_p2(&s,&r); ge_p2_dbl(&r,&s);

    ge_p1p1_to_p3(&u,&r);
    ge_select(&t, Ai, e[i]);
    ge_add(&r, &u, &t);
  }

  ge_p1p1_to_p3(h,&r);
}

