import { TestBed } from '@angular/core/testing';
import { HttpErrorResponse } from '@angular/common/http';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { AuthService, getAuthErrorMessage } from './auth.service';
import { environment } from '../../environments/environment';

describe('AuthService', () => {
  let service: AuthService;
  let httpTestingController: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(AuthService);
    httpTestingController = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpTestingController.verify();
  });

  it('rechecks authentication after a successful login', () => {
    service.checkAuth().subscribe();
    httpTestingController
      .expectOne(`${environment.apiUrl}/auth/me`)
      .flush({ status: 'failure' }, { status: 401, statusText: 'Unauthorized' });

    service.login({ email: 'user@example.com', password: 'password' }).subscribe();
    httpTestingController
      .expectOne(`${environment.apiUrl}/auth/login`)
      .flush({ status: 'success' });

    let isAuthenticated: boolean | undefined;
    service.checkAuth().subscribe((value) => (isAuthenticated = value));
    httpTestingController
      .expectOne(`${environment.apiUrl}/auth/me`)
      .flush({ status: 'success', user_id: 'user-123' });

    expect(isAuthenticated).toBeTrue();
  });

  it('extracts backend messages from auth response bodies', () => {
    expect(
      getAuthErrorMessage(
        new HttpErrorResponse({ error: { error: 'Invalid credentials' } }),
        'Login failed. Please try again.',
      ),
    ).toBe('Invalid credentials');
    expect(
      getAuthErrorMessage(
        { status: 'error', message: 'Email is already registered' },
        'Registration failed. Please try again.',
      ),
    ).toBe('Email is already registered');
  });

  it('sends authenticated password changes with the session cookie', () => {
    const request = {
      currentPassword: 'old-password',
      newPassword: 'new-password',
      confirmPassword: 'new-password',
    };
    service.changePassword(request).subscribe();

    const httpRequest = httpTestingController.expectOne(
      `${environment.apiUrl}/auth/password`,
    );
    expect(httpRequest.request.method).toBe('POST');
    expect(httpRequest.request.body).toEqual(request);
    expect(httpRequest.request.withCredentials).toBeTrue();
    httpRequest.flush({ status: 'success', message: 'password changed' });
  });

  it('requests a password reset without exposing account state', () => {
    service.requestPasswordReset('user@example.com').subscribe();
    const request = httpTestingController.expectOne(
      `${environment.apiUrl}/auth/password/forgot`,
    );
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({ email: 'user@example.com' });
    expect(request.request.withCredentials).toBeTrue();
    request.flush({ status: 'success' });
  });

  it('sends current-password confirmation with self-service account deletion', () => {
    service.deleteAccount('current-password').subscribe();
    const request = httpTestingController.expectOne(
      `${environment.apiUrl}/auth/account`,
    );
    expect(request.request.method).toBe('DELETE');
    expect(request.request.body).toEqual({ currentPassword: 'current-password' });
    request.flush({ status: 'success' });
  });
});
