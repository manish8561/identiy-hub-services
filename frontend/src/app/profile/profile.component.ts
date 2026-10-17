import { Component, OnInit } from '@angular/core';
import { Router } from '@angular/router';
import { CommonModule } from '@angular/common';
import {
  FormBuilder,
  FormGroup,
  ReactiveFormsModule,
  Validators,
} from '@angular/forms';
import { AuthService } from '../services/auth.service';
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
  errorMessage = '';
  notificationError = '';
  preference?: NotificationPreference;
  history?: NotificationHistory;
  savingPreference = false;
  editingProfile = false;
  savingProfile = false;
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
        [Validators.required, Validators.maxLength(100), Validators.pattern(/\S/)],
      ],
      email: ['', [Validators.required, Validators.email]],
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
    this.profileForm.reset({ name: this.user.name, email: this.user.email });
    this.editingProfile = true;
  }

  cancelEditingProfile(): void {
    this.editingProfile = false;
    if (this.user) {
      this.profileForm.reset({ name: this.user.name, email: this.user.email });
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

    this.savingProfile = true;
    this.userService
      .updateProfile(this.user.id, {
        name: this.profileForm.value.name.trim(),
        email: this.profileForm.value.email.trim(),
        role: this.user.role ?? 'customer',
      })
      .subscribe({
        next: (profile) => {
          this.user = profile;
          this.profileForm.reset({ name: profile.name, email: profile.email });
          this.editingProfile = false;
          this.savingProfile = false;
          this.authService.invalidateProfileCache();
          this.toastService.success('Profile updated successfully.');
        },
        error: (error: unknown) => {
          this.savingProfile = false;
          this.toastService.error(
            error instanceof Error
              ? error.message
              : 'Could not update your profile.',
          );
        },
      });
  }

  requestAccountDeletion(): void {
    this.confirmingAccountDeletion = true;
  }

  cancelAccountDeletion(): void {
    this.confirmingAccountDeletion = false;
  }

  deleteAccount(): void {
    if (!this.user || this.deletingAccount) {
      return;
    }

    this.deletingAccount = true;
    this.userService.deleteProfile(this.user.id).subscribe({
      next: () => {
        this.authService.logout().subscribe({
          next: () => this.finishAccountDeletion(),
          error: () => this.finishAccountDeletion(),
        });
      },
      error: (error: unknown) => {
        this.deletingAccount = false;
        this.confirmingAccountDeletion = false;
        this.toastService.error(
          error instanceof Error
            ? error.message
            : 'Could not delete your account. Please try again.',
        );
      },
    });
  }

  private finishAccountDeletion(): void {
    this.authService.clearSession();
    this.toastService.success('Your account has been deleted.');
    this.router.navigate(['/login']);
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
