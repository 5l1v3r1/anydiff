package anyseq

import (
	"github.com/unixpickle/anydiff"
	"github.com/unixpickle/anyvec"
)

// Reduce eliminates sequences in b to get a new batch
// with the requested present map.
//
// It is invalid for present[i] to be true when
// b.Present[i] is false.
//
// The resulting batch will have a different packed vector
// than b (even if the present map is the same).
func (b *Batch) Reduce(present []bool) *Batch {
	n := b.NumPresent()
	inc := b.Packed.Len() / n

	var chunks []anyvec.Vector
	var chunkStart, chunkSize int
	for i, pres := range present {
		if pres {
			if !b.Present[i] {
				panic("cannot re-add sequences")
			}
			chunkSize += inc
		} else if b.Present[i] {
			if chunkSize > 0 {
				chunks = append(chunks, b.Packed.Slice(chunkStart, chunkStart+chunkSize))
				chunkStart += chunkSize
				chunkSize = 0
			}
			chunkStart += inc
		}
	}
	if chunkSize > 0 {
		chunks = append(chunks, b.Packed.Slice(chunkStart, chunkStart+chunkSize))
	}

	return &Batch{
		Packed:  b.Packed.Creator().Concat(chunks...),
		Present: present,
	}
}

// Expand reverses the process of Reduce by inserting zero
// entries in the batch to get a desired present map.
//
// It is invalid for present[i] to be false when
// b.Present[i] is true.
//
// The resulting batch will have a different packed vector
// than b (even if the present map is the same).
func (b *Batch) Expand(present []bool) *Batch {
	n := b.NumPresent()
	inc := b.Packed.Len() / n
	filler := b.Packed.Creator().MakeVector(inc)

	var chunks []anyvec.Vector
	var chunkStart, chunkSize int

	for i, pres := range present {
		if b.Present[i] {
			if !pres {
				panic("argument to Expand must be a superset")
			}
			chunkSize += inc
		} else if pres {
			if chunkSize > 0 {
				chunks = append(chunks, b.Packed.Slice(chunkStart, chunkStart+chunkSize))
				chunkStart += chunkSize
				chunkSize = 0
			}
			chunks = append(chunks, filler)
		}
	}
	if chunkSize > 0 {
		chunks = append(chunks, b.Packed.Slice(chunkStart, chunkSize+chunkStart))
	}

	return &Batch{
		Packed:  b.Packed.Creator().Concat(chunks...),
		Present: present,
	}
}

type reduceRes struct {
	In  Seq
	Out []*Batch
}

// Reduce reduces all of the batches in a Seq to be
// subsets of the present list.
//
// Unlike Batch.Reduce, there is no restriction on which
// elements of present may be true.
//
// Removed sequences are kept in the Seq to preserve
// sequence indices within the batch.
// To remove these empty sequences, use Prune().
func Reduce(s Seq, present []bool) Seq {
	in := s.Output()
	res := &reduceRes{In: s, Out: make([]*Batch, len(in))}
	for i, x := range in {
		p := make([]bool, len(present))
		for i, b := range present {
			p[i] = b && x.Present[i]
		}
		res.Out[i] = x.Reduce(p)
		if res.Out[i].NumPresent() == 0 {
			res.Out = res.Out[:i]
			break
		}
	}
	return res
}

func (r *reduceRes) Creator() anyvec.Creator {
	return r.In.Creator()
}

func (r *reduceRes) Output() []*Batch {
	return r.Out
}

func (r *reduceRes) Vars() anydiff.VarSet {
	return r.In.Vars()
}

func (r *reduceRes) Propagate(u []*Batch, grad anydiff.Grad) {
	inOut := r.In.Output()
	newU := make([]*Batch, len(inOut))
	for i, x := range u {
		newU[i] = x.Expand(inOut[i].Present)
	}
	for i := len(u); i < len(inOut); i++ {
		newU[i] = &Batch{
			Packed:  inOut[i].Packed.Creator().MakeVector(inOut[i].Packed.Len()),
			Present: inOut[i].Present,
		}
	}
	r.In.Propagate(newU, grad)
}

type pruneRes struct {
	In  Seq
	Out []*Batch
}

// Prune removes all empty sequences from the batch.
func Prune(s Seq) Seq {
	sOut := s.Output()
	if len(sOut) == 0 {
		return s
	}
	out := make([]*Batch, len(sOut))
	for i, x := range sOut {
		var newPres []bool
		for j, keep := range sOut[0].Present {
			if keep {
				newPres = append(newPres, x.Present[j])
			}
		}
		out[i] = &Batch{Packed: x.Packed, Present: newPres}
	}
	return &pruneRes{In: s, Out: out}
}

func (p *pruneRes) Creator() anyvec.Creator {
	return p.In.Creator()
}

func (p *pruneRes) Output() []*Batch {
	return p.Out
}

func (p *pruneRes) Vars() anydiff.VarSet {
	return p.In.Vars()
}

func (p *pruneRes) Propagate(u []*Batch, g anydiff.Grad) {
	matchingUp := make([]*Batch, len(u))
	in := p.In.Output()
	for i, x := range u {
		matchingUp[i] = &Batch{Packed: x.Packed, Present: in[i].Present}
	}
	p.In.Propagate(matchingUp, g)
}
