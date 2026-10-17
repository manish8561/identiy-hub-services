import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';

import { DashboardComponent } from './dashboard.component';
import { AuthService } from '../services/auth.service';
import { ToastService } from '../services/toast.service';
import { UserService } from '../services/user.service';

describe('DashboardComponent', () => {
  let component: DashboardComponent;
  let fixture: ComponentFixture<DashboardComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [DashboardComponent],
      providers: [
        {
          provide: AuthService,
          useValue: { getProfile: () => of({ id: 'user-1', name: 'Test User' }) },
        },
        {
          provide: UserService,
          useValue: {
            listUsers: () =>
              of({
                users: [{ id: 'user-1', name: 'Test User', email: 'user@example.com' }],
                total: 1,
                totalCount: 1,
                page: 1,
                size: 10,
              }),
          },
        },
        { provide: ToastService, useValue: { error: jasmine.createSpy() } },
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
    expect(component.users.length).toBe(1);
    expect(component.totalUsers).toBe(1);
  });
});
