import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';
import {
  FormBuilder,
  FormGroup,
  ReactiveFormsModule,
  Validators,
} from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { AuthService, getAuthErrorMessage } from '../services/auth.service';

@Component({
  selector: 'app-reset-password',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink],
  templateUrl: './reset-password.component.html',
})
export class ResetPasswordComponent {
  readonly token: string;
  readonly passwordForm: FormGroup;
  saving = false;
  success = false;
  errorMessage = '';

  constructor(
    private readonly formBuilder: FormBuilder,
    private readonly route: ActivatedRoute,
    private readonly authService: AuthService,
  ) {
    this.token = this.route.snapshot.queryParamMap.get('token') ?? '';
    this.passwordForm = this.formBuilder.group(
      {
        newPassword: [
          '',
          [Validators.required, Validators.minLength(6), Validators.maxLength(72)],
        ],
        confirmPassword: ['', Validators.required],
      },
      {
        validators: (group) =>
          group.get('newPassword')?.value === group.get('confirmPassword')?.value
            ? null
            : { passwordMismatch: true },
      },
    );
  }

  resetPassword(): void {
    if (this.saving) {
      return;
    }
    if (!this.token) {
      this.errorMessage = 'This password reset link is missing its token.';
      return;
    }
    if (this.passwordForm.invalid) {
      this.passwordForm.markAllAsTouched();
      return;
    }

    this.saving = true;
    this.errorMessage = '';
    this.authService.resetPassword({
      token: this.token,
      newPassword: this.passwordForm.controls['newPassword'].value ?? '',
      confirmPassword: this.passwordForm.controls['confirmPassword'].value ?? '',
    }).subscribe({
      next: () => {
        this.saving = false;
        this.success = true;
      },
      error: (error: unknown) => {
        this.saving = false;
        this.errorMessage = getAuthErrorMessage(
          error,
          'This password reset link is invalid or expired.',
        );
      },
    });
  }
}
