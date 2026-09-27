// Файл: internal/recognize/locate.go
package recognize

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"math"
	"sort"

	"github.com/disintegration/imaging"

	"pylesos/internal/tables"
)

// LocateThreshold — минимальное сходство найденной карточки с образцом. Замер на скринах
// переписки пользователя: верные находки 0,73–0,96, ложная (права вместо удостоверения) 0,65.
const LocateThreshold = 0.70

const (
	locatorWidth = 160 // ширина хранимого в шаблоне образца карточки
	workWidth    = 120 // ширина страницы при поиске
)

// Match — найденная на странице карточка документа (в пикселях страницы).
type Match struct {
	Rect  image.Rectangle
	Score float64
}

// Locator — образец карточки для поиска: рамка документа, вырезанная из образца, в оттенках
// серого шириной 160 px, base64 PNG (хранится в шаблоне).
func Locator(sampleJPEG []byte, frame tables.Zone) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(sampleJPEG))
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	rect := image.Rect(int(frame.X*w), int(frame.Y*h), int((frame.X+frame.W)*w), int((frame.Y+frame.H)*h)).Add(b.Min)
	crop := imaging.Resize(imaging.Grayscale(imaging.Crop(img, rect)), locatorWidth, 0, imaging.Box)
	var buf bytes.Buffer
	if err := png.Encode(&buf, crop); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// gray — яркости изображения в виде матрицы.
func gray(img image.Image) ([]float64, int, int) {
	g := imaging.Grayscale(img)
	w, h := g.Bounds().Dx(), g.Bounds().Dy()
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out[y*w+x] = float64(g.Pix[y*g.Stride+x*4])
		}
	}
	return out, w, h
}

// Locate ищет на странице карточки, похожие на образец, на разных масштабах и позициях
// (нормированная взаимная корреляция). Повороты и перспектива не поддерживаются — фото
// документов под углом ищутся плохо и обрабатываются обычным путём.
// Нечитаемый образец или страница — карточек нет (nil).
func Locate(locator string, pageJPEG []byte) []Match {
	tpl, page, ok := decodeLocate(locator, pageJPEG)
	if !ok {
		return nil
	}
	pw, ph := page.Bounds().Dx(), page.Bounds().Dy()
	scale := float64(workWidth) / float64(pw)
	P, W, H := gray(imaging.Resize(page, workWidth, int(float64(ph)*scale+0.5), imaging.Box))
	it := newIntegral(P, W, H)
	aspect := float64(tpl.Bounds().Dy()) / float64(tpl.Bounds().Dx())
	var cands []Match
	for frac := 0.35; frac <= 1.001; frac += 0.03 {
		w := int(float64(W)*frac + 0.5)
		h := int(float64(w)*aspect + 0.5)
		if w < 20 || h < 12 || h >= H || w > W {
			continue
		}
		T, ts, ok := zeroMean(tpl, w, h)
		if !ok {
			continue
		}
		cands = append(cands, it.scan(T, ts, w, h, scale)...)
	}
	return suppress(cands)
}

func decodeLocate(locator string, pageJPEG []byte) (image.Image, image.Image, bool) {
	raw, err := base64.StdEncoding.DecodeString(locator)
	if err != nil {
		return nil, nil, false
	}
	tpl, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, false
	}
	page, _, err := image.Decode(bytes.NewReader(pageJPEG))
	if err != nil {
		return nil, nil, false
	}
	return tpl, page, true
}

// integral — страница в оттенках серого и интегральные изображения для среднего и дисперсии окна за O(1).
type integral struct {
	P       []float64
	W, H    int
	sum, sq []float64
}

func newIntegral(P []float64, W, H int) *integral {
	it := &integral{P: P, W: W, H: H, sum: make([]float64, (W+1)*(H+1)), sq: make([]float64, (W+1)*(H+1))}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			v := P[y*W+x]
			i := (y+1)*(W+1) + x + 1
			it.sum[i] = v + it.sum[i-1] + it.sum[i-(W+1)] - it.sum[i-(W+1)-1]
			it.sq[i] = v*v + it.sq[i-1] + it.sq[i-(W+1)] - it.sq[i-(W+1)-1]
		}
	}
	return it
}

func (it *integral) box(a []float64, x, y, w, h int) float64 {
	W := it.W
	return a[(y+h)*(W+1)+x+w] - a[y*(W+1)+x+w] - a[(y+h)*(W+1)+x] + a[y*(W+1)+x]
}

// zeroMean — образец w×h с нулевым средним и его стандартное отклонение; false — образец однотонный.
func zeroMean(tpl image.Image, w, h int) ([]float64, float64, bool) {
	T, _, _ := gray(imaging.Resize(tpl, w, h, imaging.Box))
	n := float64(w * h)
	var tm float64
	for _, v := range T {
		tm += v
	}
	tm /= n
	var ts float64
	for i := range T {
		T[i] -= tm
		ts += T[i] * T[i]
	}
	ts = math.Sqrt(ts / n)
	return T, ts, ts >= 1e-6
}

// scan — окна размера w×h с шагом 2, похожие на образец не меньше LocateThreshold (в пикселях страницы).
func (it *integral) scan(T []float64, ts float64, w, h int, scale float64) []Match {
	n := float64(w * h)
	var out []Match
	for y := 0; y+h <= it.H; y += 2 {
		for x := 0; x+w <= it.W; x += 2 {
			mean := it.box(it.sum, x, y, w, h) / n
			v := it.box(it.sq, x, y, w, h)/n - mean*mean
			if v < 1 {
				continue
			}
			score := it.dot(T, x, y, w, h) / (n * ts * math.Sqrt(v))
			if score >= LocateThreshold {
				out = append(out, Match{Rect: image.Rect(
					int(float64(x)/scale), int(float64(y)/scale),
					int(float64(x+w)/scale), int(float64(y+h)/scale)), Score: score})
			}
		}
	}
	return out
}

func (it *integral) dot(T []float64, x, y, w, h int) float64 {
	var dot float64
	for ty := 0; ty < h; ty++ {
		row := it.P[(y+ty)*it.W+x : (y+ty)*it.W+x+w]
		trow := T[ty*w : ty*w+w]
		for tx := range trow {
			dot += row[tx] * trow[tx]
		}
	}
	return dot
}

// suppress — подавление перекрывающихся находок: остаются лучшие, не больше четырёх.
func suppress(cands []Match) []Match {
	sort.Slice(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	var out []Match
	for _, c := range cands {
		keep := true
		for _, k := range out {
			if iou(c.Rect, k.Rect) > 0.3 {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, c)
			if len(out) == 4 {
				break
			}
		}
	}
	return out
}

func iou(a, b image.Rectangle) float64 {
	in := a.Intersect(b)
	if in.Empty() {
		return 0
	}
	ia := float64(in.Dx() * in.Dy())
	return ia / (float64(a.Dx()*a.Dy()+b.Dx()*b.Dy()) - ia)
}

// zoneInMatch переводит зону, размеченную на образце, в зону найденной карточки на странице.
func zoneInMatch(z, frame tables.Zone, m image.Rectangle, pageW, pageH int) tables.Zone {
	rx, ry := (z.X-frame.X)/frame.W, (z.Y-frame.Y)/frame.H
	rw, rh := z.W/frame.W, z.H/frame.H
	mw, mh := float64(m.Dx()), float64(m.Dy())
	return tables.Zone{
		X: (float64(m.Min.X) + rx*mw) / float64(pageW),
		Y: (float64(m.Min.Y) + ry*mh) / float64(pageH),
		W: rw * mw / float64(pageW),
		H: rh * mh / float64(pageH),
	}
}
