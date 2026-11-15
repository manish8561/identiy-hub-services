import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import {
  FormBuilder,
  FormGroup,
  ReactiveFormsModule,
  Validators,
} from '@angular/forms';
import { AuthService, getAuthErrorMessage } from '../services/auth.service';
import { Router } from '@angular/router';
import {
  NotificationHistory,
  NotificationPreference,
  NotificationService,
} from '../services/notification.service';
import { ToastService } from '../services/toast.service';
import { UserProfile, UserService } from '../services/user.service';

@Component({
  selector: 'app-profile',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './profile.component.html',
  styleUrls: ['./profile.component.scss'],
})
export class ProfileComponent implements OnInit {
  user: UserProfile | null = null;
  profileForm: FormGroup;
  passwordForm: FormGroup;
  accountDeleteForm: FormGroup;
  errorMessage = '';
  notificationError = '';
  preference?: NotificationPreference;
  history?: NotificationHistory;
  savingPreference = false;
  editingProfile = false;
  savingProfile = false;
  savingPassword = false;
  confirmingAccountDeletion = false;
  deletingAccount = false;

  get historyPageCount(): number {
    return this.history ? Math.ceil(this.history.total / this.history.size) : 0;
  }

  constructor(
    private readonly formBuilder: FormBuilder,
    private readonly authService: AuthService,
    private readonly userService: UserService,
    private readonly notificationService: NotificationService,
    private readonly toastService: ToastService,
    private readonly router: Router,
  ) {
    this.profileForm = this.formBuilder.group({
      name: [
        '',
        [
          Validators.required,
          Validators.maxLength(100),
          Validators.pattern(/\S/),
        ],
      ],
      email: ['', [Validators.required, Validators.email]],
      currentPassword: [''],
    });
    this.passwordForm = this.formBuilder.group(
      {
        currentPassword: ['', Validators.required],
        newPassword: [
          '',
          [Validators.required, Validators.minLength(6), Validators.maxLength(72)],
        ],
        confirmPassword: ['', Validators.required],
      },
      {
        validators: (group) =>
          group.get('newPassword')?.value ===
          group.get('confirmPassword')?.value
            ? null
            : { passwordMismatch: true },
      },
    );
    this.accountDeleteForm = this.formBuilder.group({
      currentPassword: ['', Validators.required],
    });
  }

  ngOnInit(): void {
    this.authService.getProfile().subscribe({
      next: (profile) => {
        this.user = profile;
        this.profileForm.patchValue({
          name: profile.name,
          email: profile.email,
        });
      },
      error: (error) => {
        this.errorMessage = 'Failed to load profile.';
        this.toastService.error(
          error.error?.error || 'Failed to load profile.',
        );
      },
    });

    this.loadNotifications();
  }

  startEditingProfile(): void {
    if (!this.user) {
      return;
    }
    this.profileForm.reset({
      name: this.user.name,
      email: this.user.email,
      currentPassword: '',
    });
    this.editingProfile = true;
  }

  cancelEditingProfile(): void {
    this.editingProfile = false;
    if (this.user) {
      this.profileForm.reset({
        name: this.user.name,
        email: this.user.email,
        currentPassword: '',
      });
    }
  }

  saveProfile(): void {
    if (!this.user || this.savingProfile) {
      return;
    }
    if (this.profileForm.invalid) {
      this.profileForm.markAllAsTouched();
      return;
    }

    const emailChanged =
      this.profileForm.value.email.trim() !== this.user.email;
    if (emailChanged && !this.profileForm.value.currentPassword) {
      this.profileForm.controls['currentPassword'].markAsTouched();
      return;
    }

    this.savingProfile = true;
    this.authService
      .updateOwnProfile({
        name: this.profileForm.value.name.trim(),
        email: this.profileForm.value.email.trim(),
        currentPassword: this.profileForm.value.currentPassword || undefined,
      })
      .subscribe({
        next: (response) => {
          this.editingProfile = false;
          this.savingProfile = false;
          this.authService.invalidateProfileCache();
          if (response.verificationRequired) {
            this.authService.clearSession();
            this.toastService.success(
              'Profile updated. Verify your new email address before signing in.',
            );
            this.router.navigate(['/login']);
            return;
          }
          const updatedProfile = {
            ...this.user!,
            name: this.profileForm.value.name.trim(),
            email: this.profileForm.value.email.trim(),
          };
          this.user = updatedProfile;
          this.profileForm.reset({
            name: this.user.name,
            email: this.user.email,
            currentPassword: '',
          });
          this.toastService.success('Profile updated successfully.');
        },
        error: (error: unknown) => {
          this.savingProfile = false;
          if (
            typeof error === 'object' &&
            error !== null &&
            'error' in error &&
            typeof error.error === 'object' &&
            error.error !== null &&
            'verificationRequired' in error.error &&
            error.error.verificationRequired === true
          ) {
            this.authService.clearSession();
            this.toastService.error(
              'Your email changed, but verification email delivery failed. Sign in later and resend verification.',
            );
            this.router.navigate(['/login']);
            return;
          }
          this.toastService.error(
            error instanceof Error
              ? error.message
              : 'Could not update your profile.',
          );
        },
      });
  }

  changePassword(): void {
    if (this.savingPassword) {
      return;
    }
    if (this.passwordForm.invalid) {
      this.passwordForm.markAllAsTouched();
      return;
    }

    this.savingPassword = true;
    this.authService.changePassword(this.passwordForm.getRawValue()).subscribe({
      next: () => {
        this.passwordForm.reset();
        this.savingPassword = false;
        this.toastService.success('Password changed successfully.');
      },
      error: (error: unknown) => {
        this.savingPassword = false;
        this.toastService.error(
          getAuthErrorMessage(error, 'Could not change your password.'),
        );
      },
    });
  }

  resendVerification(): void {
    if (!this.user) {
      return;
    }
    this.authService.resendVerification(this.user.email).subscribe({
      next: (response) =>
        this.toastService.success(response.message ?? 'Verification email sent.'),
      error: (error: unknown) =>
        this.toastService.error(
          getAuthErrorMessage(error, 'Could not resend the verification email.'),
        ),
    });
  }

  confirmAccountDeletion(): void {
    this.confirmingAccountDeletion = true;
    this.accountDeleteForm.reset();
  }

  cancelAccountDeletion(): void {
    this.confirmingAccountDeletion = false;
    this.accountDeleteForm.reset();
  }

  deleteAccount(): void {
    if (this.deletingAccount) {
      return;
    }
    if (this.accountDeleteForm.invalid) {
      this.accountDeleteForm.markAllAsTouched();
      return;
    }
    this.deletingAccount = true;
    this.authService
      .deleteAccount(this.accountDeleteForm.controls['currentPassword'].value ?? '')
      .subscribe({
        next: () => {
          this.authService.clearSession();
          this.toastService.success('Your account has been deleted.');
          this.router.navigate(['/login']);
        },
        error: (error: unknown) => {
          this.deletingAccount = false;
          this.toastService.error(
            getAuthErrorMessage(error, 'Could not delete your account.'),
          );
        },
      });
  }

  loadNotifications(page = 1): void {
    this.notificationError = '';
    this.notificationService.getPreference().subscribe({
      next: (preference) => (this.preference = preference),
      error: () => {
        this.toastService.error('Could not load email preferences.');
      },
    });
    this.notificationService.getHistory(page).subscribe({
      next: (history) => (this.history = history),
      error: () => {
        this.toastService.error('Could not load notification history.');
      },
    });
  }

  updateEmailPreference(event: Event): void {
    const enabled = (event.target as HTMLInputElement).checked;
    this.savingPreference = true;
    this.notificationError = '';
    this.notificationService.updatePreference(enabled).subscribe({
      next: (preference) => {
        this.preference = preference;
        this.savingPreference = false;
        this.toastService.success('Email preferences updated successfully!');
      },
      error: () => {
        this.toastService.error('Could not save email preferences.');
        this.savingPreference = false;
      },
    });
  }

  changeHistoryPage(page: number): void {
    if (
      !this.history ||
      page < 1 ||
      page > Math.ceil(this.history.total / this.history.size)
    ) {
      return;
    }
    this.loadNotifications(page);
  }
}
