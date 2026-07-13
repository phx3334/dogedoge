package pkg

import(
	"math"
	"math/rand"
	"fmt"
	"time"
)

func GenerateRandomCode(length int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("%0*d", length, r.Intn(int(math.Pow10(length))))
}
