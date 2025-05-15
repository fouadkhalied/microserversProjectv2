package infrastructure

import (
    "fmt"
    "crypto/rand"
	"crypto/subtle"
    "time"
    "context"
    "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"os"
	"log"
	"errors"
	"math/big"
)

type OTPService struct {
    EMAIL_API_KEY string
    EMAIL_SENDER string
}

func NewOTPService() *OTPService {
    return &OTPService{
        EMAIL_API_KEY: os.Getenv("EMAIL_API_KEY"),
        EMAIL_SENDER: os.Getenv("EMAIL_SENDER"),
    }
}

// Exported function
func (o *OTPService) SendOTP(ctx context.Context,recipientEmail string, otp string) error {
	from := mail.NewEmail("Real state", o.EMAIL_SENDER)
	subject := "Your OTP Code"
	to := mail.NewEmail("", recipientEmail)

	plainTextContent := fmt.Sprintf("Your OTP code is: %s", otp)
	htmlContent := fmt.Sprintf("<strong>Your OTP code is: %s</strong>", otp)

	message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)
	client := sendgrid.NewSendClient(os.Getenv("EMAIL_API_KEY"))
	response, err := client.Send(message)

	if err != nil {
		log.Println("Failed to send OTP email:", err)
		return err
	}

	fmt.Println("Email sent. Status Code:", response.StatusCode)
	return nil
}


func (o *OTPService) GenerateOTP(ctx context.Context) string {
    // Generate a 6-digit OTP using crypto/rand
    otp := make([]byte, 6)
    for i := range otp {
        n, err := rand.Int(rand.Reader, big.NewInt(10))
        if err != nil {
            // Fallback in case of error
            return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
        }
        otp[i] = byte(n.Int64()) + '0'
    }
    return string(otp)
}

func (o * OTPService) VerifyOTP(ctx context.Context, email, providedOTP , cacheOtp string) (bool, error) {
	
	isValid := subtle.ConstantTimeCompare([]byte(cacheOtp), []byte(providedOTP)) == 1
	
	if isValid {
		// Delete the OTP after successful verification to prevent reuse
			return true, nil
	}
	return false, errors.New("wrong OTP verification") 
}