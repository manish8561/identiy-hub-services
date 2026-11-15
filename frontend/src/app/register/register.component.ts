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
  RegisterRequest,
} from '../services/auth.service';
import { ToastService } from '../services/toast.service';

@Component({
  selector: 'app-register',
  standalone: true,
  imports: [ReactiveFormsModule, CommonModule, RouterModule],
  templateUrl: './register.component.html',
  styles: [
    `
      .register-container {
        max-width: 400px;
        margin: 2rem auto;
        padding: 2rem;
        border-radius: 8px;
        box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
        background-color: white;
      }

      .register-card {
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

      .login-link {
        text-align: center;
        margin-top: 1rem;
        color: #4a5568;
      }

      .login-link a {
        color: #3b82f6;
        text-decoration: none;
        font-weight: 500;
      }

      .login-link a:hover {
        text-decoration: underline;
      }
    `,
  ],
})
export class RegisterComponent implements OnInit {
  registerForm: FormGroup;
  errorMessage: string = '';
  registrationMessage = '';

  constructor(
    private fb: FormBuilder,
    private authService: AuthService,
    private router: Router,
    private changeDetector: ChangeDetectorRef,
    private toastService: ToastService,
  ) {
    this.registerForm = this.fb.group(
      {
        name: ['', Validators.required],
        email: ['', [Validators.required, Validators.email]],
        password: ['', [Validators.required, Validators.minLength(6)]],
        confirmPassword: ['', Validators.required],
      },
      {
        validator: this.passwordMatchValidator,
      },
    );
  }

  ngOnInit(): void {
    this.authService.checkAuth().subscribe((isLoggedIn) => {
      if (isLoggedIn) {
        this.router.navigate(['/dashboard']);
      }
    });
  }

  passwordMatchValidator(form: FormGroup) {
    const password = form.get('password');
    const confirmPassword = form.get('confirmPassword');
    if (
      password &&
      confirmPassword &&
      password.value !== confirmPassword.value
    ) {
      return { mismatch: true };
    }
    return null;
  }

  onSubmit() {
    if (this.registerForm.valid) {
      this.errorMessage = '';
      this.registrationMessage = '';
      this.changeDetector.markForCheck();
      const userData: RegisterRequest = this.registerForm.value;
      this.authService.register(userData).subscribe({
        next: (response) => {
          if (response.status === 'success') {
            this.registrationMessage =
              response.message ??
              'Registration successful. Check your email to verify your account.';
            this.registerForm.reset();
          } else {
            this.toastService.error(
              getAuthErrorMessage(
                response,
                'Registration failed. Please try again.',
              ),
            );
          }
        },
        error: (error: unknown) => {
          this.toastService.error(
            getAuthErrorMessage(
              error,
              'Registration failed. Please try again.',
            ),
          );
        },
      });
    }
  }

  private showError(message: string): void {
    this.errorMessage = message;
    this.changeDetector.markForCheck();
  }
}
