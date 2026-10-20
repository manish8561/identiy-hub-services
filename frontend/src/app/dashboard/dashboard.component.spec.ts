import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';

import { DashboardComponent } from './dashboard.component';
import { AuthService } from '../services/auth.service';
import { ToastService } from '../services/toast.service';
import { UserService } from '../services/user.service';

describe('DashboardComponent', () => {
  let component: DashboardComponent;
  let fixture: ComponentFixture<DashboardComponent>;
  const profile = {
    id: 'admin-1',
    name: 'Admin User',
    email: 'admin@example.com',
    role: 'admin',
  };
  const listedUser = {
    id: 'user-1',
    name: 'Test User',
    email: 'user@example.com',
    role: 'customer',
    createdAt: '',
    updatedAt: '',
  };
  const authService = {
    getProfile: jasmine.createSpy().and.returnValue(of(profile)),
  };
  const userService = {
    listUsers: jasmine.createSpy().and.returnValue(
      of({
        users: [listedUser],
        total: 1,
        totalCount: 1,
        page: 1,
        size: 10,
      }),
    ),
    updateProfile: jasmine.createSpy().and.returnValue(of(listedUser)),
    deleteProfile: jasmine
      .createSpy()
      .and.returnValue(of({ message: 'deleted' })),
  };
  const toastService = {
    error: jasmine.createSpy(),
    success: jasmine.createSpy(),
  };

  beforeEach(async () => {
    authService.getProfile.and.returnValue(of(profile));
    userService.listUsers.calls.reset();
    userService.updateProfile.calls.reset();
    userService.deleteProfile.calls.reset();
    toastService.error.calls.reset();
    toastService.success.calls.reset();

    await TestBed.configureTestingModule({
      imports: [DashboardComponent],
      providers: [
        { provide: AuthService, useValue: authService },
        { provide: UserService, useValue: userService },
        { provide: ToastService, useValue: toastService },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(DashboardComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('loads users from the user API', () => {
    expect(userService.listUsers).toHaveBeenCalledWith(1, 10);
    expect(component.users.length).toBe(1);
    expect(component.totalUsers).toBe(1);
  });

  it('updates a listed user through the user API', () => {
    component.startEditingUser(listedUser);
    component.userForm.setValue({
      name: 'Updated User',
      email: 'updated@example.com',
      role: 'admin',
    });

    component.saveUser(listedUser);

    expect(userService.updateProfile).toHaveBeenCalledWith('user-1', {
      name: 'Updated User',
      email: 'updated@example.com',
      role: 'admin',
    });
    expect(toastService.success).toHaveBeenCalledWith(
      'User updated successfully.',
    );
  });

  it('deletes a listed user through the user API after confirmation', () => {
    component.requestUserDeletion(listedUser.id);
    component.deleteUser(listedUser);

    expect(userService.deleteProfile).toHaveBeenCalledWith('user-1');
    expect(toastService.success).toHaveBeenCalledWith(
      'User deleted successfully.',
    );
  });

  it('does not allow a non-admin to initiate user deletion', () => {
    component.isAdmin = false;

    component.requestUserDeletion(listedUser.id);
    component.deleteUser(listedUser);

    expect(component.confirmingDeleteUserId).toBeNull();
    expect(userService.deleteProfile).not.toHaveBeenCalled();
  });

  it('does not load the admin user list for a customer', () => {
    authService.getProfile.and.returnValue(
      of({ ...profile, role: 'customer' }),
    );
    userService.listUsers.calls.reset();

    const customerFixture = TestBed.createComponent(DashboardComponent);
    customerFixture.detectChanges();

    expect(customerFixture.componentInstance.isAdmin).toBeFalse();
    expect(userService.listUsers).not.toHaveBeenCalled();
  });
});
