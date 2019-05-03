package entropy

import (
	"crypto/rand"
	"fmt"
)

func GetRandom(len uint32) ([]byte, error) {
	random := make([]byte, len)
	err := customEntropy(random)
	if err != nil {
		// If customEntropy provides an error, fallback to go crypto/rand generator.
		_, err := rand.Read(random)
		return random, err
	}
	return random, nil
}

func customEntropy(random []byte) (error) {
	// Stub: return error because no custom entropy has been implemented yet.
	return fmt.Errorf("no custom source of entropy has been implemented yet")

	/* An example of providing randomness from an fake http endpoint:

	url := fmt.Sprintf("https://randomness-source.com/entropy?bytes=%d", bytes)
	resp, errGetRandomness := http.Get(url)
	if errGetRandomness != nil {
		return nil, errGetRandomness
	}

	body, errReadBody := ioutil.ReadAll(resp.Body)
	if errReadBody != nil {
		return nil, errReadBody
	}

	return body, nil

	*/
}