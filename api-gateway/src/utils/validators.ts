// api-gateway/src/utils/validators.ts

/**
 * Validates user registration data
 * @param userData User registration data
 * @returns Array of validation error messages (empty if valid)
 */
export function validateUserRegistration(userData: any): string[] {
    const errors: string[] = [];
    
    // Check required fields
    if (!userData.username) {
      errors.push('Username is required');
    } else if (userData.username.length < 3 || userData.username.length > 50) {
      errors.push('Username must be between 3 and 50 characters');
    }
    
    if (!userData.password) {
      errors.push('Password is required');
    } else if (userData.password.length < 8) {
      errors.push('Password must be at least 8 characters long');
    }
    
    if (!userData.email) {
      errors.push('Email is required');
    } else if (!isValidEmail(userData.email)) {
      errors.push('Email format is invalid');
    }
    
    return errors;
  }
  
  /**
   * Validates user login data
   * @param userData User login data
   * @returns Array of validation error messages (empty if valid)
   */
  export function validateUserLogin(userData: any): string[] {
    const errors: string[] = [];
    
    if (!userData.username) {
      errors.push('Username is required');
    }
    
    if (!userData.password) {
      errors.push('Password is required');
    }
    
    return errors;
  }
  
  /**
   * Validates an email address format
   * @param email Email address to validate
   * @returns Boolean indicating if the email format is valid
   */
  function isValidEmail(email: string): boolean {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
  }
  
  /**
   * Sanitizes a string to prevent injection attacks
   * @param input String to sanitize
   * @returns Sanitized string
   */
  export function sanitizeInput(input: string): string {
    // Basic sanitization - in production, consider using a library like DOMPurify
    return input
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#x27;')
      .replace(/\//g, '&#x2F;');
  }
  
  /**
   * Validates a token format (basic check)
   * @param token Authentication token
   * @returns Boolean indicating if the token format seems valid
   */
  export function isValidToken(token: string): boolean {
    // JWT tokens are typically 3 base64url sections separated by dots
    const parts = token.split('.');
    return parts.length === 3 && parts.every(part => part.length > 0);
  }