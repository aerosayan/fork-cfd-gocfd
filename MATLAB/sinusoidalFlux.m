syms a b c d x y gamma
%2D Polynomial field
rho(x,y)=a*x/2*pi+b*y/2*pi;
u = c*sin(x/2*pi); v = d*cos(y/2*pi);
rhou=rho*u; rhov=rho*v;
p=rho^gamma;
q=0.5*rho*(u^2+v^2);
E=p/(gamma-1)+q;
U = [ rho, rhou, rhov, E];
F = [ rhou, rho*u^2+p, rho*u*v, u*(E+p) ];
G = [ rhov, rho*u*v, rho*v^2+p, v*(E+p) ];
div = diff(F,x)+diff(G,y);
fprintf('Code for Divergence of F and G Fluxes\n%s\n',ccode(div));
fprintf('Code for U \n%s\n%s\n%s\n%s\n',ccode(U));
plotRho(x,y) = subs(rho,[a,b],[1.,1.]);
plotDiv(x,y) = subs(div,[a,b,c,d,gamma],[1.,1.,1.,1.,1.4]);
x = linspace(-2*pi,2*pi,20);
y = linspace(0,4*pi,20);
[X,Y] = meshgrid(x,y);
Z = plotDiv(X,Y);
figure('Name','Divergence of Rho');
contour(X,Y,double(Z{1}),10, 'ShowText','on');
%Divergence of this field is complex DOH!
% figure('Name','Divergence of RhoU');
% contour(X,Y,double(Z{2}),10, 'ShowText','on');