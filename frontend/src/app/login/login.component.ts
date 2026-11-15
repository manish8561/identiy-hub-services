import { ChangeDetectorRef, Component, OnInit } from '@angular/core';
import { Router, RouterModule } from '@angular/router';
import {
  FormBuilder,
  FormGroup,
  Validators,
  ReactiveFormsModule,
} from '@angular/forms';
import { CommonModule } from '@angular/common';
import {
  AuthService,
  getAuthErrorMessage,
  LoginRequest,
} from '../services/auth.service';
import { ToastService } from '../services/toast.service';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [ReactiveFormsModule, CommonModule, RouterModule],
  templateUrl: './login.component.html',
  styles: [
    `
      .login-container {
        max-width: 400px;
        margin: 2rem auto;
        padding: 2rem;
        border-radius: 8px;
        box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
        background-color: white;
      }

      .login-card {
        display: flex;
        flex-direction: column;
        gap: 1.5rem;
      }

      h2 {
        text-align: center;
        color: #1a1a1a;
        margin-bottom: 1rem;
      }

      form {
        display: flex;
        flex-direction: column;
        gap: 1rem;
      }

      .form-group {
        display: flex;
        flex-direction: column;
        gap: 0.5rem;
      }

      label {
        font-weight: 500;
        color: #4a5568;
      }

      input {
        padding: 0.75rem;
        border: 1px solid #e2e8f0;
        border-radius: 4px;
        font-size: 1rem;
        transition: border-color 0.2s;
      }

      input:focus {
        outline: none;
        border-color: #3b82f6;
        box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
      }

      input.ng-invalid.ng-touched {
        border-color: #ef4444;
      }

      .error-message {
        color: #ef4444;
        font-size: 0.875rem;
        margin-top: 0.25rem;
      }

      button {
        padding: 0.75rem;
        background-color: #3b82f6;
        color: white;
        border: none;
        border-radius: 4px;
        font-size: 1rem;
        font-weight: 500;
        cursor: pointer;
        transition: background-color 0.2s;
      }

      button:hover {
        background-color: #2563eb;
      }

      button:disabled {
        background-color: #93c5fd;
        cursor: not-allowed;
      }

      .register-link {
        text-align: center;
        margin-top: 1rem;
        color: #4a5568;
      }

      .register-link a {
        color: #3b82f6;
        text-decoration: none;
        font-weight: 500;
      }

      .register-link a:hover {
        text-decoration: underline;
      }
    `,
  ],
})
export class LoginComponent implements OnInit {
  loginForm: FormGroup;
  errorMessage: string = '';
  actionMessage = '';
  resetRequested = false;
  verificationResent = false;

  constructor(
    private fb: FormBuilder,
    private authService: AuthService,
    private router: Router,
    private changeDetector: ChangeDetectorRef,
    private toastService: ToastService,
  ) {
    this.loginForm = this.fb.group({
      email: ['', [Validators.required, Validators.email]],
      password: ['', [Validators.required, Validators.minLength(6)]],
    });
  }

  ngOnInit(): void {
    this.authService.checkAuth().subscribe((isLoggedIn) => {
      if (isLoggedIn) {
        this.router.navigate(['/dashboard']);
      }
    });
  }

  onSubmit() {
    if (this.loginForm.valid) {
      this.errorMessage = '';
      this.actionMessage = '';
      this.changeDetector.markForCheck();
      const credentials: LoginRequest = this.loginForm.value;
      this.authService.login(credentials).subscribe({
        next: (response) => {
          if (response.status === 'success') {
            this.toastService.success('Login successful!');
            this.router.navigate(['/dashboard']);
          } else {
            this.toastService.error(
              getAuthErrorMessage(response, 'Login failed. Please try again.'),
            );
          }
        },
        error: (error: unknown) => {
          this.toastService.error(
            getAuthErrorMessage(error, 'Login failed. Please try again.'),
          );
        },
      });
    }
  }

  requestPasswordReset(): void {
    const email = this.loginForm.controls['email'].value as string;
    if (this.loginForm.controls['email'].invalid) {
      this.loginForm.controls['email'].markAsTouched();
      return;
    }
    this.authService.requestPasswordReset(email).subscribe({
      next: () => {
        this.actionMessage =
          'If an account exists for that email, password reset instructions have been sent.';
        this.resetRequested = true;
      },
      error: (error: unknown) => {
        this.toastService.error(
          getAuthErrorMessage(error, 'Could not request a password reset.'),
        );
      },
    });
  }

  resendVerification(): void {
    const email = this.loginForm.controls['email'].value as string;
    if (this.loginForm.controls['email'].invalid) {
      this.loginForm.controls['email'].markAsTouched();
      return;
    }
    this.authService.resendVerification(email).subscribe({
      next: () => {
        this.actionMessage =
          'If the account needs verification, a verification email has been sent.';
        this.verificationResent = true;
      },
      error: (error: unknown) => {
        this.toastService.error(
          getAuthErrorMessage(error, 'Could not resend the verification email.'),
        );
      },
    });
  }

  private showError(message: string): void {
    this.errorMessage = message;
    this.changeDetector.markForCheck();
  }
}
