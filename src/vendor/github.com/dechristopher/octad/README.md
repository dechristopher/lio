# octad
[![GoDoc](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](https://godoc.org/github.com/dechristopher/octad)
[![Go Report Card](https://goreportcard.com/badge/notnil/chess)](https://goreportcard.com/report/dechristopher/octad)
[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/dechristopher/octad/master/LICENSE)

**octad** is a set of go packages which provide common octad chess variant
utilities such as move generation, turn management, checkmate detection,
a basic engine, PGN encoding, image generation, and others.

## Repo Structure

| Package    | Docs Link | Description |
| ---------- | -------------------------------------------- | --------------------------------------------------------------------------------------- |
| **octad**  | [dechristopher/octad](README.md)             | Move generation, serialization / deserialization, turn management, checkmate detection  |
| **image**  | [dechristopher/octad/image](image/README.md) | SVG octad board image generation                                                        |
| **liad**   | [dechristopher/octad/liad](liad/liad.go)     | **Li**(bre oct)**ad** test harness and sample self play implementation                  |


## Installation

**octad** can be installed using "go get".

```bash
go get -u github.com/dechristopher/octad
``` 

## Octad Game
Octad was conceived by Andrew DeChristopher in 2018. Rules and information about
the game can be found below. Octad is thought to be a solved, deterministic game
but needs formal verification to prove that. This repository exists as an effort
towards that goal.

### Board Layout
Each player begins with four pieces: a knight, their king, and two pawns placed
in that order from left to right relative to them. An example of this can be
seen in the board diagrams below:

| Octad Board | 1. c2 | 1. c2 b3 | 1. c2 b3 2. cxb3! ... |
| ----------- | ----- | -------- | --------------------- |
| ![Octad board](doc/octad1.svg "Octad board") | ![Octad board](doc/octad2.svg "1. c2") | ![Octad board](doc/octad3.svg "1. c2 b3") | ![Octad board](doc/octad4.svg "1. c2 b3 2. cxb3! ...") |

### Rules
All standard chess rules apply:

* En passant is allowed
* Pawn promotion to any piece
* Stalemates are a draw

The only catch, however, is that castling is possible between the king and any
of its pieces on the starting rank before movement. The king will simply switch
spaces with the castling piece in all cases except the far pawn, in which case
the king will travel one space to the right, and the pawn will lie where the
king was before. An example of white castling with their far pawn can be
expressed as `[ 1. c2 b3 2. O-O-O ... ]` with the resulting structure leaving
the knight on a1, a pawn on b1, the king on c1, and the other pawn on c2. Here
is what that would look like on the board:

![Octad board](doc/far-castle.svg "White after far pawn castling")

#### Castling Notation
* Knight castle: **O**
* Close pawn castle: **O-O**
* Far pawn castle: **O-O-O**

### OFEN Notation
OFEN is a derivation of FEN to support the features of Octad. Read more
[here](doc/OFEN.md).

### Sample Games

```pgn
1.O-O a3
2.Nc2 a2
3.b3+ Nxb3+
4.Kb2 a1=Q+
5.Nxa1 Nxa1
6.Kxa1 Kc3
7.Ka2 b3+
8.Ka1 b2+
9.Kb1 Kb3
10.d3 Kc3
11.d4=Q+ Kxd4
12.Kxb2 1/2-1/2

Drawn by Insufficient Material
```

```pgn
1.c2 b3
2.Kb2 O-O-O
3.cxb3 cxb3
4.d2 Nc2
5.d3 Nxa1
6.d4=Q# 1-0

White wins by Checkmate
```

## Performance

**octad** has been performance tuned, using
[pprof](https://golang.org/pkg/runtime/pprof/), with the goal of being fast
enough for use by octad bots and engines. This implementation relies heavily
on the use of [bitboards](https://chessprogramming.wikispaces.com/Bitboards),
resulting in very solid computational performance.

### Benchmarks

The benchmarks can be run with the following command:
```
go test -bench=.
```

Results from the baseline 2019 16" MBP:
```
BenchmarkBitboardReverse-12           	1000000000     0.000016 ns/op
BenchmarkStalemateStatus-12           	971688	       1220 ns/op
BenchmarkInvalidStalemateStatus-12    	1387780	       857 ns/op
BenchmarkPositionHash-12              	1429471	       841 ns/op
BenchmarkValidMoves-12                	235640	       4992 ns/op
```