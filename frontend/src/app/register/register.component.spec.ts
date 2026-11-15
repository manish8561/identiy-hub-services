import { HttpErrorResponse } from '@angular/common/http';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { AuthService } from '../services/auth.service';
import { ToastService } from '../services/toast.service';
import { RegisterComponent } from './register.component';

describe('RegisterComponent', () => {
  let component: RegisterComponent;
  let fixture: ComponentFixture<RegisterComponent>;
  let authService: jasmine.SpyObj<AuthService>;
  let toastService: jasmine.SpyObj<ToastService>;

  beforeEach(async () => {
    authService = jasmine.createSpyObj<AuthService>('AuthService', [
      'checkAuth',
      'register',
    ]);
    authService.checkAuth.and.returnValue(of(false));
    toastService = jasmine.createSpyObj<ToastService>('ToastService', [
      'error',
      'success',
      'warning',
    ]);

    await TestBed.configureTestingModule({
      imports: [RegisterComponent],
      providers: [
        { provide: AuthService, useValue: authService },
        { provide: ToastService, useValue: toastService },
        provideRouter([]),
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(RegisterComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('displays the backend error after a failed registration', () => {
    authService.register.and.returnValue(
      throwError(
        () =>
          new HttpErrorResponse({
            status: 400,
            error: { error: 'user already exists' },
          }),
      ),
    );
    component.registerForm.setValue({
      name: 'Test User',
      email: 'user@example.com',
      password: 'password',
      confirmPassword: 'password',
    });

    component.onSubmit();
    expect(toastService.error).toHaveBeenCalledWith('user already exists');
  });
});
