Point(1) = {0, 0, 0, 0.002};
Point(2) = {0.5, 0, 0, 0.002};
Point(3) = {1.0, 0, 0, 0.002};
Point(4) = {1.0, 0.01, 0, 0.002};
Point(5) = {0.5, 0.01, 0, 0.002};
Point(6) = {0, 0.01, 0, 0.002};
Line(1) = {1, 2};
Line(2) = {2, 5};
Line(3) = {5, 6};
Line(4) = {6, 1};
Line(5) = {2, 3};
Line(6) = {3, 4};
Line(7) = {4, 5};
Curve Loop(1) = {1, 2, 3, 4};
Curve Loop(2) = {5, 6, 7, -2};
Plane Surface(1) = {1};
Plane Surface(2) = {2};
Physical Curve("In") = {4};
Physical Curve("Out") = {6};
Physical Curve("Wall") = {3, 1, 7, 5};