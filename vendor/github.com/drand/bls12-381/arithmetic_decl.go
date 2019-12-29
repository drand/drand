package bls

//go:noescape
func add6(c, a, b *fe)

//go:noescape
func add_assign_6(a, b *fe)

//go:noescape
func ladd6(c, a, b *fe)

//go:noescape
func ladd_assign_6(a, b *fe)

//go:noescape
func add12(c, a, b *lfe)

//go:noescape
func add_assign_12(a, b *lfe)

//go:noescape
func ladd12(c, a, b *lfe)

//go:noescape
func double6(c, a *fe)

//go:noescape
func double_assign_6(a *fe)

//go:noescape
func ldouble6(c, a *fe)

//go:noescape
func double12(c, a *lfe)

//go:noescape
func double_assign_12(a *lfe)

//go:noescape
func ldouble12(c, a *lfe)

//go:noescape
func sub6(c, a, b *fe)

//go:noescape
func sub_assign_6(a, b *fe)

//go:noescape
func lsub6(c, a, b *fe)

//go:noescape
func lsub_assign_nc_6(a, b *fe)

//go:noescape
func sub12(c, a, b *lfe)

//go:noescape
func sub_assign_12(a, b *lfe)

//go:noescape
func lsub12(c, a, b *lfe)

//go:noescape
func lsub_assign_12(a, b *lfe)

//go:noescape
func sub12_opt1_h2(c, a, b *lfe)

//go:noescape
func sub12_opt1_h1(c, a, b *lfe)

//go:noescape
func neg(c, a *fe)

//go:noescape
func mul_nobmi2(c *lfe, a, b *fe)

//go:noescape
func mont_nobmi2(c *fe, a *lfe)

//go:noescape
func montmul_nobmi2(c, a, b *fe)

//go:noescape
func montmul_assign_nobmi2(a, b *fe)

//go:noescape
func mul_bmi2(c *lfe, a, b *fe)

//go:noescape
func mont_bmi2(c *fe, a *lfe)

//go:noescape
func montmul_bmi2(c, a, b *fe)

//go:noescape
func montmul_assign_bmi2(a, b *fe)
