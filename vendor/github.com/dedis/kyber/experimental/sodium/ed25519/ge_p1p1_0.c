#include "ge.h"

/*
r = 0
*/

extern void ge_p1p1_0(ge_p3 *r)
{
  fe_0(r->X);
  fe_1(r->Y);
  fe_1(r->Z);
  fe_1(r->T);
}

