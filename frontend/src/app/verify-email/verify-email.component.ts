import { Component, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { AuthService, getAuthErrorMessage } from '../services/auth.service';

@Component({
  selector: 'app-verify-email',
  standalone: true,
  imports: [CommonModule, RouterLink],
  templateUrl: './verify-email.component.html',
})
export class VerifyEmailComponent implements OnInit {
  verifying = true;
  message = '';
  error = '';

  constructor(
    private readonly route: ActivatedRoute,
    private readonly authService: AuthService,
  ) {}

  ngOnInit(): void {
    const token = this.route.snapshot.queryParamMap.get('token');
    if (!token) {
      this.verifying = false;
      this.error = 'This email verification link is missing its token.';
      return;
    }

    this.authService.verifyEmail(token).subscribe({
      next: () => {
        this.verifying = false;
        this.message = 'Your email is verified. You can now sign in.';
      },
      error: (error: unknown) => {
        this.verifying = false;
        this.error = getAuthErrorMessage(error, 'This link is invalid or expired.');
      },
    });
  }
}
