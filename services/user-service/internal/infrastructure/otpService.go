package infrastructure

import (
    "fmt"
    "crypto/rand"
    "user-service/internal/repository"
    "time"
    "context"
    "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"os"
	"log"
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
        EMAIL_API_KEY: os.Getenv("EMAIL_API_KEY"),
        EMAIL_SENDER: os.Getenv("EMAIL_SENDER"),
    }
}

// Exported function
func (o *OTPService) SendOTP(ctx context.Context,recipientEmail string, otp string) error {
	from := mail.NewEmail("Real state", "foukha49@gmail.com")
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


func (o * OTPService) GenerateOTP(ctx context.Context) string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("%06d", int(b[0])<<16|int(b[1])<<8|int(b[2])%1000000)
}

func (o * OTPService) VerifyOTP(ctx context.Context, email, providedOTP , cacheOtp string) (bool, error) {
	
	isValid := cacheOtp == providedOTP
	
	if isValid {
		// Delete the OTP after successful verification to prevent reuse
		//if err := o.redisRepo.DeleteKey(ctx, "otp:"+email); err != nil {
			return true, nil // OTP was valid but failed to delete
		//}
	}
	
	return isValid, nil
}