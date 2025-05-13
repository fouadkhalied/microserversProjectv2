package infrastructure

import (
    "fmt"
    "crypto/rand"
    "user-service/internal/repository"
    "time"
    "context"
    "net/http"
    "io/ioutil"
    "encoding/json"
    "bytes"
)

type OTPService struct {
    EMAIL_API_KEY string
    EMAIL_SENDER string
    redisRepo   *repository.RedisRepo
    otpLength   int
	otpExpiry   time.Duration
}

func NewOTPService() *OTPService {
    return &OTPService{
        EMAIL_API_KEY: "0fe7f9bc96f9a92a9ed8a1e95de20eeb",
        EMAIL_SENDER: "no-reply@github.com",
    }
}
// Exported function
func (o *OTPService) SendOTP(recipientEmail string) error {
	otp := o.generateOTP()

	payload := map[string]interface{}{
		"from": map[string]string{
			"email": "hello@example.com",
			"name":  "Mailtrap Test",
		},
		"to": []map[string]string{
			{
				"email": recipientEmail,
			},
		},
		"subject":  "Your OTP Code",
		"text":     fmt.Sprintf("Your OTP is: %s", otp),
		"category": "OTP",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshalling payload: %w", err)
	}

    fmt.Print(recipientEmail)

	req, err := http.NewRequest("POST", "https://sandbox.api.mailtrap.io/api/send/3692272", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer 0fe7f9bc96f9a92a9ed8a1e95de20eeb")
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	fmt.Println("Mailtrap response:", string(body))
	return nil
}

func (o * OTPService) generateOTP() string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("%06d", int(b[0])<<16|int(b[1])<<8|int(b[2])%1000000)
}

func (r * OTPService) VerifyOTP(ctx context.Context, email, providedOTP string) (bool, error) {
	storedOTP, err := r.redisRepo.GetOTP(ctx, email)
	if err != nil {
		return false, err
	}
	
	if storedOTP == "" {
		return false, nil // OTP not found or expired
	}
	
	isValid := storedOTP == providedOTP
	
	if isValid {
		// Delete the OTP after successful verification to prevent reuse
		if err := r.redisRepo.DeleteKey(ctx, "otp:"+email); err != nil {
			return true, err // OTP was valid but failed to delete
		}
	}
	
	return isValid, nil
}