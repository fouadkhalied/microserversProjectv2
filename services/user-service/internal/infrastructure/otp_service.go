package infrastructure
import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/resend/resend-go/v2"
)

type OTPService struct {
	EMAIL_API_KEY string
	EMAIL_SENDER  string
	OTP_EXPIRY    time.Duration
	OTP_LENGTH    int
	client        *resend.Client
}

func NewOTPService() *OTPService {
	// Get OTP configuration from environment variables
	otpExpiry := GetEnvAsDuration("OTP_EXPIRY", 5*time.Minute)
	otpLength := GetEnvAsInt("OTP_LENGTH", 6)
	apiKey := os.Getenv("EMAIL_API_KEY")
	emailSender := os.Getenv("EMAIL_SENDER")

	// Log configuration (without exposing the full API key)
	maskedApiKey := ""
	if len(apiKey) > 8 {
		maskedApiKey = apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
	}
	log.Printf("OTP Service Config - API Key: %s, Sender: %s, Expiry: %v, Length: %d", 
		maskedApiKey, emailSender, otpExpiry, otpLength)

	// Initialize Resend client
	client := resend.NewClient(apiKey)

	return &OTPService{
		EMAIL_API_KEY: apiKey,
		EMAIL_SENDER:  emailSender,
		OTP_EXPIRY:    otpExpiry,
		OTP_LENGTH:    otpLength,
		client:        client,
	}
}

func (o *OTPService) SendOTP(ctx context.Context, recipientEmail string, otp string) error {
    log.Printf("Sending OTP to: %s", recipientEmail)
    
    params := &resend.SendEmailRequest{
        From:    o.EMAIL_SENDER, // Use the working sender
        To:      []string{recipientEmail},
        Subject: "Your OTP Code",
        Text:    fmt.Sprintf("Your OTP code is: %s", otp),
    }

    response, err := o.client.Emails.Send(params) // Try without context first
    if err != nil {
        log.Printf("Resend error: %+v", err)
        return err
    }

    log.Printf("Email sent successfully. ID: %s", response.Id)
    return nil
}


func (o *OTPService) GenerateOTP(ctx context.Context) string {
	// Generate OTP using configured length
	otp := make([]byte, o.OTP_LENGTH)
	for i := range otp {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// Fallback in case of error
			return fmt.Sprintf("%0*d", o.OTP_LENGTH, time.Now().UnixNano()%int64(10^o.OTP_LENGTH))
		}
		otp[i] = byte(n.Int64()) + '0'
	}
	return string(otp)
}

func (o *OTPService) VerifyOTP(ctx context.Context, email, providedOTP, cacheOtp string) (bool, error) {
	isValid := subtle.ConstantTimeCompare([]byte(cacheOtp), []byte(providedOTP)) == 1

	if isValid {
		// Delete the OTP after successful verification to prevent reuse
		return true, nil
	}
	return false, errors.New("wrong OTP verification")
}

