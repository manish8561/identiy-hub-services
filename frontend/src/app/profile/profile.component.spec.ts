import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';

import { ProfileComponent } from './profile.component';
import { AuthService } from '../services/auth.service';
import { NotificationService } from '../services/notification.service';
import { ToastService } from '../services/toast.service';
import { UserService } from '../services/user.service';

describe('ProfileComponent', () => {
  let component: ProfileComponent;
  let fixture: ComponentFixture<ProfileComponent>;
  const profile = {
    id: 'user-1',
    name: 'Test User',
    email: 'user@example.com',
    role: 'customer',
    createdAt: '',
    updatedAt: '',
  };
  const authService = {
    getProfile: () => of(profile),
    invalidateProfileCache: jasmine.createSpy(),
  };
  const userService = {
    updateProfile: jasmine.createSpy().and.returnValue(of(profile)),
  };
  beforeEach(async () => {
    authService.invalidateProfileCache.calls.reset();
    userService.updateProfile.calls.reset();

    await TestBed.configureTestingModule({
      imports: [ProfileComponent],
      providers: [
        { provide: AuthService, useValue: authService },
        { provide: UserService, useValue: userService },
        {
          provide: ToastService,
          useValue: {
            error: jasmine.createSpy(),
            success: jasmine.createSpy(),
          },
        },
        {
          provide: NotificationService,
          useValue: {
            getPreference: () =>
              of({ userId: 'user-1', emailEnabled: true, updatedAt: '' }),
            getHistory: () => of({ items: [], total: 0, page: 1, size: 10 }),
            updatePreference: () =>
              of({ userId: 'user-1', emailEnabled: true, updatedAt: '' }),
          },
        },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(ProfileComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('updates the authenticated user profile', () => {
    component.startEditingProfile();
    component.profileForm.setValue({
      name: 'Updated Name',
      email: 'updated@example.com',
    });

    component.saveProfile();

    expect(userService.updateProfile).toHaveBeenCalledWith('user-1', {
      name: 'Updated Name',
      email: 'updated@example.com',
      role: 'customer',
    });
    expect(authService.invalidateProfileCache).toHaveBeenCalled();
    expect(component.editingProfile).toBeFalse();
  });

  it('does not expose account deletion to an admin', () => {
    component.user = { ...profile, role: 'admin' };
    fixture.detectChanges();

    const deleteButtons = Array.from(
      fixture.nativeElement.querySelectorAll('button'),
    ).filter((button: HTMLButtonElement) =>
      /delete/i.test(button.textContent ?? ''),
    );

    expect(deleteButtons.length).toBe(0);
  });
});
