import { Component, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { AuthService } from '../services/auth.service';
import { CommonModule } from '@angular/common';
import { ToastService } from '../services/toast.service';
import {
  FormBuilder,
  FormGroup,
  ReactiveFormsModule,
  Validators,
} from '@angular/forms';
import {
  UserListResponse,
  UserProfile,
  UserService,
} from '../services/user.service';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink],
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.scss'],
})
export class DashboardComponent implements OnInit {
  user: UserProfile | null = null;
  isAdmin = false;
  users: UserProfile[] = [];
  page = 1;
  readonly pageSize = 10;
  totalUsers = 0;
  loadingUsers = false;
  userListError = '';
  editingUserId: string | null = null;
  savingUserId: string | null = null;
  confirmingDeleteUserId: string | null = null;
  deletingUserId: string | null = null;
  readonly userForm: FormGroup;

  get pageCount(): number {
    return Math.ceil(this.totalUsers / this.pageSize) || 1;
  }

  constructor(
    private readonly formBuilder: FormBuilder,
    private readonly authService: AuthService,
    private readonly userService: UserService,
    private readonly toastService: ToastService,
  ) {
    this.userForm = this.formBuilder.group({
      name: [
        '',
        [Validators.required, Validators.maxLength(100), Validators.pattern(/\S/)],
      ],
      email: ['', [Validators.required, Validators.email]],
      role: ['customer', Validators.required],
    });
  }

  ngOnInit(): void {
    this.authService.getProfile().subscribe({
      next: (profile: UserProfile) => {
        this.user = profile;
        this.isAdmin = profile.role === 'admin';
        if (this.isAdmin) {
          this.loadUsers();
        }
      },
      error: (error) => {
        console.error('Profile load error:', error);
        this.toastService.error(
          error.error?.error || 'Failed to load profile.',
        );
      },
    });
  }

  loadUsers(page = this.page): void {
    if (!this.isAdmin) {
      return;
    }
    this.page = page;
    this.loadingUsers = true;
    this.userListError = '';
    this.userService.listUsers(this.page, this.pageSize).subscribe({
      next: (response: UserListResponse) => {
        const total = response.totalCount ?? response.total ?? 0;
        const lastPage = Math.ceil(total / this.pageSize);
        if (this.page > Math.max(lastPage, 1)) {
          this.loadUsers(Math.max(lastPage, 1));
          return;
        }
        this.users = response.users ?? [];
        this.totalUsers = total;
        this.loadingUsers = false;
      },
      error: (error: unknown) => {
        this.userListError =
          error instanceof Error
            ? error.message
            : 'Could not load users. Please try again.';
        this.loadingUsers = false;
        this.toastService.error(this.userListError);
      },
    });
  }

  startEditingUser(user: UserProfile): void {
    if (!this.isAdmin) {
      return;
    }
    this.confirmingDeleteUserId = null;
    this.editingUserId = user.id;
    this.userForm.reset({
      name: user.name,
      email: user.email,
      role: user.role ?? 'customer',
    });
  }

  cancelEditingUser(): void {
    this.editingUserId = null;
  }

  saveUser(user: UserProfile): void {
    if (!this.isAdmin || this.savingUserId) {
      return;
    }
    if (this.userForm.invalid) {
      this.userForm.markAllAsTouched();
      return;
    }

    this.savingUserId = user.id;
    this.userService
      .updateProfile(user.id, {
        name: this.userForm.value.name.trim(),
        email: this.userForm.value.email.trim(),
        role: this.userForm.value.role,
      })
      .subscribe({
        next: () => {
          this.savingUserId = null;
          this.editingUserId = null;
          this.toastService.success('User updated successfully.');
          this.loadUsers(this.page);
        },
        error: (error: unknown) => {
          this.savingUserId = null;
          this.toastService.error(
            error instanceof Error
              ? error.message
              : 'Could not update this user. Please try again.',
          );
        },
      });
  }

  requestUserDeletion(userId: string): void {
    if (!this.isAdmin) {
      return;
    }
    this.editingUserId = null;
    this.confirmingDeleteUserId = userId;
  }

  cancelUserDeletion(): void {
    this.confirmingDeleteUserId = null;
  }

  deleteUser(user: UserProfile): void {
    if (!this.isAdmin || this.deletingUserId) {
      return;
    }

    this.deletingUserId = user.id;
    this.userService.deleteProfile(user.id).subscribe({
      next: () => {
        this.deletingUserId = null;
        this.confirmingDeleteUserId = null;
        this.toastService.success('User deleted successfully.');
        this.loadUsers(this.page);
      },
      error: (error: unknown) => {
        this.deletingUserId = null;
        this.confirmingDeleteUserId = null;
        this.toastService.error(
          error instanceof Error
            ? error.message
            : 'Could not delete this user. Please try again.',
        );
      },
    });
  }

  changePage(page: number): void {
    if (page < 1 || page > this.pageCount) {
      return;
    }
    this.loadUsers(page);
  }
}
