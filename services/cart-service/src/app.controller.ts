import { Controller } from '@nestjs/common';
import { EventPattern, Payload } from '@nestjs/microservices';

@Controller()
export class AppController {
  @EventPattern('cart.created')
  handleCartCreated(@Payload() data: string) {
    console.log('🛒 Received cart.created:', data);
  }
}
