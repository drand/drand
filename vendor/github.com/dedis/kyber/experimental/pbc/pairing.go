// +build experimental
// +build pbc

// Package pbc provides a Go wrapper for
// the Stanford Pairing Based Crypto library.
package pbc

// #include <stdlib.h>
// #include <pbc/pbc.h>
// #cgo LDFLAGS: -lpbc -lgmp
import "C"

import (
	"runtime"
	"unsafe"

	"github.com/dedis/kyber/abstract"
)

type g1group struct{ p *Pairing }
type g2group struct{ p *Pairing }
type gtgroup struct{ p *Pairing }

// A Pairing object represents a pairing-based cryptography environment,
// consisting of two source groups G1 and G2 and a target group GT.
// All of these groups support the standard Group API operations.
// In addition, the GT group supports the new Pairing operation,
// via the PairingPoint extension to the Point interface.
// The input groups G1 and G2 may be identical or different,
// as indicated by the Symmetric() method.
type Pairing struct {
	p    C.pairing_t
	name string
	g1   g1group
	g2   g2group
	gt   gtgroup
}

// Group interface extension to create pairing-capable points.
type PairingGroup interface {
	abstract.Group // Standard Group operations

	PairingPoint() PairingPoint // Create new pairing-capable Point
}

// Point interface extension for a point in a pairing target group (GT),
// which supports the Pairing operation.
type PairingPoint interface {
	abstract.Point // Standard Point operations

	// Compute the pairing of two points p1 and p2,
	// which must be in the associated groups G1 and G2 respectively.
	Pairing(p1, p2 abstract.Point) abstract.Point
}

func clearPairing(p *Pairing) {
	println("clearPairing", p)
	C.pairing_clear(&p.p[0])
}

// Initigalize with a given name and pairing parameter string.
// See the PBC library for parameter format and meaning.
func (p *Pairing) Init(name string, param string) *Pairing {

	cparam := C.CString(param)
	err := C.pairing_init_set_str(&p.p[0], cparam)
	C.free(unsafe.Pointer(cparam))
	if err != 0 {
		panic("Invalid pairing parameters")
	}
	runtime.SetFinalizer(p, clearPairing)

	p.name = name
	p.g1.p = p
	p.g2.p = p
	p.gt.p = p
	return p
}

// Initialize the standard D159 pairing.
func (p *Pairing) InitD159() *Pairing {
	return p.Init("D159",
		`type d
q 625852803282871856053922297323874661378036491717
n 625852803282871856053923088432465995634661283063
h 3
r 208617601094290618684641029477488665211553761021
a 581595782028432961150765424293919699975513269268
b 517921465817243828776542439081147840953753552322
k 6
nk 60094290356408407130984161127310078516360031868417968262992864809623507269833854678414046779817844853757026858774966331434198257512457993293271849043664655146443229029069463392046837830267994222789160047337432075266619082657640364986415435746294498140589844832666082434658532589211525696
hk 1380801711862212484403205699005242141541629761433899149236405232528956996854655261075303661691995273080620762287276051361446528504633283152278831183711301329765591450680250000592437612973269056
coeff0 472731500571015189154958232321864199355792223347
coeff1 352243926696145937581894994871017455453604730246
coeff2 289113341693870057212775990719504267185772707305
nqr 431211441436589568382088865288592347194866189652
`)
}

// Initialize the standard D201 pairing.
func (p *Pairing) InitD201() *Pairing {
	return p.Init("D201",
		`type d
q 2094476214847295281570670320144695883131009753607350517892357
n 2094476214847295281570670320143248652598286201895740019876423
h 1122591
r 1865751832009427548920907365321162072917283500309320153
a 9937051644888803031325524114144300859517912378923477935510
b 6624701096592535354217016076096200573011941585948985290340
k 6
nk 84421409121513221644716967251498543569964760150943970280296295496165154657097987617093928595467244393873913569302597521196137376192587250931727762632568620562823714441576400096248911214941742242106512149305076320555351603145285797909942596124862593877499051211952936404822228308154770272833273836975042632765377879565229109013234552083886934379264203243445590336
hk 24251848326363771171270027814768648115136299306034875585195931346818912374815385257266068811350396365799298585287746735681314613260560203359251331805443378322987677594618057568388400134442772232086258797844238238645130212769322779762522643806720212266304
coeff0 362345194706722765382504711221797122584657971082977778415831
coeff1 856577648996637037517940613304411075703495574379408261091623
coeff2 372728063705230489408480761157081724912117414311754674153886
nqr 279252656555925299126768437760706333663688384547737180929542
`)
}

// Initialize the standard D224 pairing.
func (p *Pairing) InitD224() *Pairing {
	return p.Init("D224",
		`type d
q 15028799613985034465755506450771565229282832217860390155996483840017
n 15028799613985034465755506450771561352583254744125520639296541195021
h 1
r 15028799613985034465755506450771561352583254744125520639296541195021
a 1871224163624666631860092489128939059944978347142292177323825642096
b 9795501723343380547144152006776653149306466138012730640114125605701
k 6
nk 11522474695025217370062603013790980334538096429455689114222024912184432319228393204650383661781864806076247259556378350541669994344878430136202714945761488385890619925553457668158504202786580559970945936657636855346713598888067516214634859330554634505767198415857150479345944721710356274047707536156296215573412763735135600953865419000398920292535215757291539307525639675204597938919504807427238735811520
hk 51014915936684265604900487195256160848193571244274648855332475661658304506316301006112887177277345010864012988127829655449256424871024500368597989462373813062189274150916552689262852603254011248502356041206544262755481779137398040376281542938513970473990787064615734720
coeff0 11975189258259697166257037825227536931446707944682470951111859446192
coeff1 13433042200347934827742738095249546804006687562088254057411901362771
coeff2 8327464521117791238079105175448122006759863625508043495770887411614
nqr 142721363302176037340346936780070353538541593770301992936740616924
`)
}

// Return the G1 source group for this pairing.
func (p *Pairing) G1() abstract.Group {
	return &p.g1
}

// Return the G2 source group for this pairing.
func (p *Pairing) G2() abstract.Group {
	return &p.g2
}

// Return the target group GT for this pairing.
func (p *Pairing) GT() PairingGroup {
	return &p.gt
}

// Return true if source groups G1 and G2 are identical in this pairing,
// meaning that Point objects taken from G1 and G2 are interchangeable.
func (p *Pairing) Symmetric() bool {
	return C.pairing_is_symmetric(&p.p[0]) != 0
}

// G1 group

func (g *g1group) String() string {
	return g.p.name + ".G1"
}

func (g *g1group) ScalarLen() int {
	return int(C.pairing_length_in_bytes_Zr(&g.p.p[0]))
}

func (g *g1group) Scalar() abstract.Scalar {
	s := newScalar()
	C.element_init_Zr(&s.e[0], &g.p.p[0])
	return s
}

func (g *g1group) PointLen() int {
	return int(C.pairing_length_in_bytes_compressed_G1(&g.p.p[0]))
}

func (g *g1group) Point() abstract.Point {
	p := newCurvePoint()
	C.element_init_G1(&p.e[0], &g.p.p[0])
	return p
}

func (g *g1group) PrimeOrder() bool {
	return true
}

// G2 group

func (g *g2group) String() string {
	return g.p.name + ".G2"
}

func (g *g2group) ScalarLen() int {
	return int(C.pairing_length_in_bytes_Zr(&g.p.p[0]))
}

func (g *g2group) Scalar() abstract.Scalar {
	s := newScalar()
	C.element_init_Zr(&s.e[0], &g.p.p[0])
	return s
}

func (g *g2group) PointLen() int {
	return int(C.pairing_length_in_bytes_compressed_G2(&g.p.p[0]))
}

func (g *g2group) Point() abstract.Point {
	p := newCurvePoint()
	C.element_init_G2(&p.e[0], &g.p.p[0])
	return p
}

func (g *g2group) PrimeOrder() bool {
	return true
}

// GT group

func (g *gtgroup) String() string {
	return g.p.name + ".GT"
}

func (g *gtgroup) ScalarLen() int {
	return int(C.pairing_length_in_bytes_Zr(&g.p.p[0]))
}

func (g *gtgroup) Scalar() abstract.Scalar {
	s := newScalar()
	C.element_init_Zr(&s.e[0], &g.p.p[0])
	return s
}

func (g *gtgroup) PointLen() int {
	return int(C.pairing_length_in_bytes_GT(&g.p.p[0]))
}

func (g *gtgroup) Point() abstract.Point {
	return g.PairingPoint()
}

func (g *gtgroup) PairingPoint() PairingPoint {
	p := newIntPoint()
	C.element_init_GT(&p.e[0], &g.p.p[0])
	return p
}

func (g *gtgroup) PrimeOrder() bool {
	return true
}
