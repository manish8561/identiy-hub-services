import { Injectable } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Observable, throwError } from 'rxjs';
import { catchError } from 'rxjs/operators';
import { environment } from '../../environments/environment';

export interface UserProfile {
  id: string;
  name: string;
  email: string;
  role?: string;
  createdAt: string;
  updatedAt: string;
}

export interface UserListResponse {
  users: UserProfile[] | null;
  total: number;
  totalCount?: number;
  page: number;
  size: number;
}

export interface UpdateUserRequest {
  name: string;
  email: string;
  role: string;
}

@Injectable({
  providedIn: 'root'
})
export class UserService {
  private apiUrl = environment.apiUrl;
  private readonly httpOptions = { withCredentials: true };

  constructor(private http: HttpClient) { }

  private handleError(error: HttpErrorResponse) {
    let errorMessage = 'An error occurred';
    if (error.error instanceof ErrorEvent) {
      errorMessage = `Error: ${error.error.message}`;
    } else {
      const responseMessage =
        typeof error.error === 'string'
          ? error.error
          : error.error?.message || error.error?.error;
      errorMessage = `Error Code: ${error.status}\nMessage: ${responseMessage || error.message}`;
    }
    console.error(errorMessage);
    return throwError(() => new Error(errorMessage));
  }

  getProfile(userId: string): Observable<UserProfile> {
    return this.http.get<UserProfile>(
      `${this.apiUrl}/users/profile/${userId}`,
      this.httpOptions
    ).pipe(
      catchError(this.handleError)
    );
  }

  getCurrentUserProfile(): Observable<UserProfile> {
    return this.http.get<UserProfile>(
      `${this.apiUrl}/users/me`,
      this.httpOptions
    ).pipe(
      catchError(this.handleError)
    );
  }

  listUsers(page: number = 1, size: number = 10): Observable<UserListResponse> {
    return this.http.get<UserListResponse>(
      `${this.apiUrl}/users/list`,
      {
        ...this.httpOptions,
        params: { page: page.toString(), size: size.toString() }
      }
    ).pipe(
      catchError(this.handleError)
    );
  }

  updateProfile(userId: string, profileData: UpdateUserRequest): Observable<UserProfile> {
    return this.http.put<UserProfile>(
      `${this.apiUrl}/users/profile/${userId}`,
      profileData,
      this.httpOptions
    ).pipe(
      catchError(this.handleError)
    );
  }

  deleteProfile(userId: string): Observable<{ message: string }> {
    return this.http.delete<{ message: string }>(
      `${this.apiUrl}/users/profile/${userId}`,
      this.httpOptions
    ).pipe(
      catchError(this.handleError)
    );
  }
}
