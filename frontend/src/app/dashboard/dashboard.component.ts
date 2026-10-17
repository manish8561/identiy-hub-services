import { Component, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { AuthService } from '../services/auth.service';
import { CommonModule } from '@angular/common';
import { ToastService } from '../services/toast.service';
import {
  UserListResponse,
  UserProfile,
  UserService,
} from '../services/user.service';

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [CommonModule, RouterLink],
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.scss'],
})
export class DashboardComponent implements OnInit {
  user: UserProfile | null = null;
  users: UserProfile[] = [];
  page = 1;
  readonly pageSize = 10;
  totalUsers = 0;
  loadingUsers = false;
  userListError = '';

  get pageCount(): number {
    return Math.ceil(this.totalUsers / this.pageSize);
  }

  constructor(
    private readonly authService: AuthService,
    private readonly userService: UserService,
    private readonly toastService: ToastService,
  ) {}

  ngOnInit(): void {
    this.authService.getProfile().subscribe({
      next: (profile: UserProfile) => (this.user = profile),
      error: (error) => {
        console.error('Profile load error:', error);
        this.toastService.error(
          error.error?.error || 'Failed to load profile.',
        );
      },
    });
    this.loadUsers();
  }

  loadUsers(page = this.page): void {
    this.page = page;
    this.loadingUsers = true;
    this.userListError = '';
    this.userService.listUsers(this.page, this.pageSize).subscribe({
      next: (response: UserListResponse) => {
        this.users = response.users ?? [];
        this.totalUsers = response.totalCount ?? response.total;
        this.loadingUsers = false;
      },
      error: () => {
        this.userListError = 'Could not load users. Please try again.';
        this.loadingUsers = false;
        this.toastService.error(this.userListError);
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
