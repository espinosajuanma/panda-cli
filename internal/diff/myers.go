package diff

type Line struct {
	Marker byte
	Text   string
}

// Myers returns a shortest edit script between before and after.
func Myers(before, after []string) []Line {
	n, m := len(before), len(after)
	maxDistance := n + m
	v := map[int]int{1: 0}
	trace := make([]map[int]int, 0, maxDistance+1)
	for distance := 0; distance <= maxDistance; distance++ {
		state := make(map[int]int, len(v))
		for diagonal, x := range v {
			state[diagonal] = x
		}
		trace = append(trace, state)
		for diagonal := -distance; diagonal <= distance; diagonal += 2 {
			var x int
			if diagonal == -distance || (diagonal != distance && v[diagonal-1] < v[diagonal+1]) {
				x = v[diagonal+1]
			} else {
				x = v[diagonal-1] + 1
			}
			y := x - diagonal
			for x < n && y < m && before[x] == after[y] {
				x++
				y++
			}
			v[diagonal] = x
			if x >= n && y >= m {
				return backtrack(trace, before, after, distance)
			}
		}
	}
	return nil
}

func backtrack(trace []map[int]int, before, after []string, distance int) []Line {
	x, y := len(before), len(after)
	reversed := make([]Line, 0, x+y)
	for d := distance; d >= 0; d-- {
		state := trace[d]
		diagonal := x - y
		var previousDiagonal int
		if diagonal == -d || (diagonal != d && state[diagonal-1] < state[diagonal+1]) {
			previousDiagonal = diagonal + 1
		} else {
			previousDiagonal = diagonal - 1
		}
		previousX := state[previousDiagonal]
		previousY := previousX - previousDiagonal
		for x > previousX && y > previousY {
			x--
			y--
			reversed = append(reversed, Line{Marker: ' ', Text: before[x]})
		}
		if d == 0 {
			break
		}
		if x == previousX {
			y--
			reversed = append(reversed, Line{Marker: '+', Text: after[y]})
		} else {
			x--
			reversed = append(reversed, Line{Marker: '-', Text: before[x]})
		}
	}
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed
}
